package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const dcsOntologyIRI = "https://w3id.org/facis/dcs/ontology/v1#"

// canonicalNamespaceMap returns the display prefix→namespace map used to compact
// IRIs in the rendered PDF (glossary terms, ontology links).
func canonicalNamespaceMap() map[string]string {
	return map[string]string{
		"dcterms": "http://purl.org/dc/terms/",
		"schema":  "https://schema.org/",
		"prov":    "http://www.w3.org/ns/prov#",
	}
}

// expandCanonicalIRI resolves a compact IRI from the canonical compact form back
// to its full absolute IRI. Handles the prefixes declared in the canonical
// context (prov:, schema:, dcterms:, xsd:) plus bare names expanded via @vocab.
// Full IRIs (http/https/urn) are returned unchanged.
func expandCanonicalIRI(compact string) string {
	switch {
	case strings.HasPrefix(compact, "prov:"):
		return "http://www.w3.org/ns/prov#" + compact[5:]
	case strings.HasPrefix(compact, "schema:"):
		return "https://schema.org/" + compact[7:]
	case strings.HasPrefix(compact, "dcterms:"):
		return "http://purl.org/dc/terms/" + compact[8:]
	case strings.HasPrefix(compact, "xsd:"):
		return "http://www.w3.org/2001/XMLSchema#" + compact[4:]
	case strings.HasPrefix(compact, "http://"), strings.HasPrefix(compact, "https://"), strings.HasPrefix(compact, "urn:"):
		return compact
	}
	if !strings.Contains(compact, ":") {
		return dcsOntologyIRI + compact
	}
	return compact
}

// parseCanonicalSegment maps one ContentItem from the canonical compact form to
// a clauseSegment. The compact form uses short term names and prefix notation
// for IRIs; expandCanonicalIRI resolves them to full IRIs where needed.
func parseCanonicalSegment(item ContentItem) clauseSegment {
	// Value objects: typed literal or plain string.
	if item.Value != "" {
		if item.Datatype != "" {
			return clauseSegment{Type: "typed-value", Value: item.Value, Datatype: expandCanonicalIRI(item.Datatype)}
		}
		return clauseSegment{Type: "prose", Text: item.Value}
	}

	// Decode raw JSON for schema: properties (schema:url, schema:name).
	var raw map[string]any
	if len(item.Raw) > 0 && item.Raw[0] == '{' {
		json.Unmarshal(item.Raw, &raw) //nolint:errcheck // best-effort parse for segment properties
	}

	// External link: object carries schema:url.
	if urlVal, ok := raw["schema:url"]; ok {
		href := ""
		switch v := urlVal.(type) {
		case string:
			href = v
		case map[string]any:
			if s, ok := v["@value"].(string); ok {
				href = s
			} else if s, ok := v["@id"].(string); ok {
				href = s
			}
		}
		text := href
		if name, ok := raw["schema:name"].(string); ok && name != "" {
			text = name
		}
		return clauseSegment{Type: "external-link", Text: text, Href: href}
	}

	// IRI reference: object carries @id.
	if item.ID != "" {
		fullIRI := expandCanonicalIRI(item.ID)
		text := fullIRI
		if raw != nil {
			if name, ok := raw["schema:name"].(string); ok && name != "" {
				text = name
			}
		}
		return clauseSegment{Type: "ontology-link", Text: text, Ref: fullIRI}
	}

	return clauseSegment{Type: "prose"}
}

func parseCanonicalClause(block *Block) clauseData {
	clause := clauseData{Segments: []clauseSegment{}}
	for _, rawItem := range block.Content {
		var item ContentItem
		if err := json.Unmarshal(rawItem, &item); err == nil {
			clause.Segments = append(clause.Segments, parseCanonicalSegment(item))
		}
	}
	return clause
}

func walkCanonicalSectionNode(sec sectionData, ln *LayoutNode, layoutByID map[string]*LayoutNode, blockByID map[string]*Block) sectionData {
	for _, childID := range ln.Children {
		block := blockByID[childID]
		if block == nil {
			continue
		}
		switch block.Type {
		case "Clause", "TextBlock":
			sec.Clauses = append(sec.Clauses, parseCanonicalClause(block))
		case "Section":
			sub := sectionData{
				Heading: strings.TrimSpace(block.Title),
				Clauses: []clauseData{},
			}
			if subLayout, ok := layoutByID[childID]; ok {
				sub = walkCanonicalSectionNode(sub, subLayout, layoutByID, blockByID)
			}
			sec.Subsections = append(sec.Subsections, sub)
		}
	}
	return sec
}

func walkCanonicalSections(ds *DocumentStructure) []sectionData {
	blockByID := make(map[string]*Block, len(ds.Blocks))
	for i := range ds.Blocks {
		b := &ds.Blocks[i]
		if b.ID != "" {
			blockByID[b.ID] = b
		}
	}

	layoutByID := make(map[string]*LayoutNode, len(ds.Layout))
	var rootLayout *LayoutNode
	for i := range ds.Layout {
		ln := &ds.Layout[i]
		if ln.IsRoot {
			rootLayout = ln
		} else if ln.ID != "" {
			layoutByID[ln.ID] = ln
		}
	}

	if rootLayout == nil {
		return nil
	}

	var sections []sectionData
	for _, childID := range rootLayout.Children {
		block := blockByID[childID]
		if block == nil {
			continue
		}
		if block.Type == "Section" {
			sec := sectionData{
				Heading: strings.TrimSpace(block.Title),
				Clauses: []clauseData{},
			}
			if secLayout, ok := layoutByID[childID]; ok {
				sec = walkCanonicalSectionNode(sec, secLayout, layoutByID, blockByID)
			}
			sections = append(sections, sec)
		}
	}
	return sections
}

// extractDocumentModelFromCanonical builds a documentModel from a canonical
// compact JSON-LD payload produced by CanonicalizePayload. It unmarshals the
// JSON into typed Go structs and traverses the layout tree to assemble sections,
// clauses, and signature fields without accessing the JSON-LD expanded form.
func extractDocumentModelFromCanonical(canonical []byte, hashHex string) (documentModel, error) {
	var tmpl ContractTemplate
	if err := json.Unmarshal(canonical, &tmpl); err != nil {
		return documentModel{}, fmt.Errorf("unmarshal canonical payload: %w", err)
	}

	model := documentModel{
		Sections:        []sectionData{},
		SignatureFields: []sigFieldDef{},
		Glossary:        []glossaryTerm{},
		NamespaceMap:    canonicalNamespaceMap(),
		CanonicalJSON:   canonical,
		PayloadHash:     hashHex,
		FileID:          hashHex[:32],
		ContractID:      tmpl.ID,
	}

	if tmpl.Metadata == nil || strings.TrimSpace(tmpl.Metadata.Title) == "" {
		return model, fmt.Errorf("metadata.title is required but was not found in the payload")
	}
	model.Title = strings.TrimSpace(tmpl.Metadata.Title)

	if tmpl.DocumentStructure != nil {
		model.Sections = walkCanonicalSections(tmpl.DocumentStructure)
	}

	for _, sf := range tmpl.SignatureFields {
		name := strings.TrimSpace(sf.SignatoryName)
		if name == "" {
			continue
		}
		label := strings.TrimSpace(sf.Title)
		if label == "" {
			label = name
		}
		model.SignatureFields = append(model.SignatureFields, sigFieldDef{Name: name, Label: label})
	}

	// compact ontology-link text when no name was provided
	var compactLinks func([]sectionData)
	compactLinks = func(sections []sectionData) {
		for si := range sections {
			for ci := range sections[si].Clauses {
				for segi := range sections[si].Clauses[ci].Segments {
					seg := &sections[si].Clauses[ci].Segments[segi]
					if seg.Type == "ontology-link" && seg.Text == seg.Ref && seg.Ref != "" {
						seg.Text = compactIRI(seg.Ref, model.NamespaceMap)
					}
				}
			}
			compactLinks(sections[si].Subsections)
		}
	}
	compactLinks(model.Sections)

	seen := make(map[string]struct{})
	var collectRefs func([]sectionData)
	collectRefs = func(sections []sectionData) {
		for _, section := range sections {
			for _, clause := range section.Clauses {
				for _, seg := range clause.Segments {
					if seg.Type == "ontology-link" && seg.Ref != "" {
						if _, ok := seen[seg.Ref]; !ok {
							seen[seg.Ref] = struct{}{}
							model.Glossary = append(model.Glossary, glossaryTerm{
								Term:    compactIRI(seg.Ref, model.NamespaceMap),
								TermURI: seg.Ref,
							})
						}
					}
				}
			}
			collectRefs(section.Subsections)
		}
	}
	collectRefs(model.Sections)

	return model, nil
}

func compactIRI(iri string, nsMap map[string]string) string {
	prefixes := make([]string, 0, len(nsMap))
	for p := range nsMap {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		if strings.HasPrefix(iri, nsMap[prefix]) {
			return prefix + ":" + strings.TrimPrefix(iri, nsMap[prefix])
		}
	}
	return iri
}

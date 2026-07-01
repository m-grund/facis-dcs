package compiler

import (
	"fmt"
	"sort"
	"strings"
)

const dcsOntologyIRI = "https://w3id.org/facis/dcs/ontology/v1#"

const (
	dcsMetadataIRI        = dcsOntologyIRI + "metadata"
	dcsTitleIRI           = dcsOntologyIRI + "title"
	dcsDocStructIRI       = dcsOntologyIRI + "documentStructure"
	dcsBlocksIRI          = dcsOntologyIRI + "blocks"
	dcsLayoutIRI          = dcsOntologyIRI + "layout"
	dcsIsRootIRI          = dcsOntologyIRI + "isRoot"
	dcsChildrenIRI        = dcsOntologyIRI + "children"
	dcsContentIRI         = dcsOntologyIRI + "content"
	dcsSectionTypeIRI     = dcsOntologyIRI + "Section"
	dcsClauseTypeIRI      = dcsOntologyIRI + "Clause"
	dcsTextBlockTypeIRI   = dcsOntologyIRI + "TextBlock"
	dcsSignatureFieldsIRI  = dcsOntologyIRI + "signatureFields"
	dcsSignatoryNameIRI    = dcsOntologyIRI + "signatoryName"
)

var (
	modelUnitCodeIRIs    = []string{"https://schema.org/unitCode", "http://schema.org/unitCode"}
	modelLinkNameIRIs    = []string{"https://schema.org/name", "http://schema.org/name"}
	modelExternalURLIRIs = []string{"https://schema.org/url", "http://schema.org/url"}
)

// extractDocumentModel builds a documentModel from the JSON-LD expanded form.
func extractDocumentModel(expanded []any, rootID string, rawCtx map[string]any, canonical []byte, hashHex string) (documentModel, error) {
	model := documentModel{
		Sections:        []sectionData{},
		SignatureFields: []sigFieldDef{},
		Glossary:        []glossaryTerm{},
		NamespaceMap:    make(map[string]string),
		CanonicalJSON:   canonical,
		PayloadHash:     hashHex,
		FileID:          hashHex[:32],
		ContractID:      rootID,
	}

	for prefix, uri := range rawCtx {
		if uriStr, ok := uri.(string); ok && !strings.HasPrefix(prefix, "@") {
			model.NamespaceMap[prefix] = uriStr
		}
	}

	if len(expanded) == 0 {
		return model, fmt.Errorf("metadata.title is required but the payload expanded to an empty graph")
	}
	root, ok := findExpandedRootNode(expanded, rootID)
	if !ok {
		return model, fmt.Errorf("metadata.title is required but no root node was found in the payload")
	}

	metaItems := ldGet(root, dcsMetadataIRI)
	if len(metaItems) == 0 {
		return model, fmt.Errorf("metadata is required but was not found in the payload")
	}
	metaNode, ok := metaItems[0].(map[string]any)
	if !ok {
		return model, fmt.Errorf("metadata must be an object")
	}
	title := ldFirstString(metaNode, dcsTitleIRI)
	if title == "" {
		return model, fmt.Errorf("metadata.title is required but was not found in the payload")
	}
	model.Title = title

	dsItems := ldGet(root, dcsDocStructIRI)
	if len(dsItems) > 0 {
		if dsNode, ok := dsItems[0].(map[string]any); ok {
			blockByID := make(map[string]map[string]any)
			for _, item := range ldGetList(dsNode, dcsBlocksIRI) {
				node, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if id, ok := node["@id"].(string); ok && id != "" {
					blockByID[id] = node
				}
			}

			layoutByID := make(map[string]map[string]any)
			var rootLayout map[string]any
			for _, item := range ldGetList(dsNode, dcsLayoutIRI) {
				node, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if nodeIsRoot(node) {
					rootLayout = node
				} else if id, ok := node["@id"].(string); ok && id != "" {
					layoutByID[id] = node
				}
			}

			// After an expand→compact→expand cycle, shared-@id nodes (appearing
			// in both blocks and layout) may be compacted to a bare {"@id": "..."}
			// reference at one of the two positions while the full merged node
			// stays at the other. Resolve bare references in each map from the other.
			for id, block := range blockByID {
				if len(ldGet(block, "@type")) == 0 {
					if lb, ok := layoutByID[id]; ok && len(ldGet(lb, "@type")) > 0 {
						blockByID[id] = lb
					}
				}
			}
			for id, ln := range layoutByID {
				if len(ldGet(ln, "@type")) == 0 {
					if blk, ok := blockByID[id]; ok && len(ldGet(blk, "@type")) > 0 {
						layoutByID[id] = blk
					}
				}
			}

			if rootLayout != nil {
				model.Sections = walkSections(rootLayout, layoutByID, blockByID)
			}
		}
	}

	for _, item := range ldGet(root, dcsSignatureFieldsIRI) {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if val, ok := node["@value"].(string); ok {
			if name := strings.TrimSpace(val); name != "" {
				model.SignatureFields = append(model.SignatureFields, sigFieldDef{Name: name, Label: name})
			}
			continue
		}
		sigName := strings.TrimSpace(ldFirstString(node, dcsSignatoryNameIRI))
		if sigName == "" {
			continue
		}
		label := strings.TrimSpace(ldFirstString(node, dcsTitleIRI))
		if label == "" {
			label = sigName
		}
		model.SignatureFields = append(model.SignatureFields, sigFieldDef{Name: sigName, Label: label})
	}

	var compactSectionLinks func(sections []sectionData)
	compactSectionLinks = func(sections []sectionData) {
		for si := range sections {
			for ci := range sections[si].Clauses {
				for segi := range sections[si].Clauses[ci].Segments {
					seg := &sections[si].Clauses[ci].Segments[segi]
					if seg.Type == "ontology-link" && seg.Text == seg.Ref && seg.Ref != "" {
						seg.Text = compactIRI(seg.Ref, model.NamespaceMap)
					}
				}
			}
			compactSectionLinks(sections[si].Subsections)
		}
	}
	compactSectionLinks(model.Sections)

	seen := make(map[string]struct{})
	var collectSectionRefs func(sections []sectionData)
	collectSectionRefs = func(sections []sectionData) {
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
			collectSectionRefs(section.Subsections)
		}
	}
	collectSectionRefs(model.Sections)

	return model, nil
}

// walkSections builds top-level sectionData from a root layout node's children.
func walkSections(rootLayout map[string]any, layoutByID map[string]map[string]any, blockByID map[string]map[string]any) []sectionData {
	var sections []sectionData
	for _, v := range ldGetList(rootLayout, dcsChildrenIRI) {
		childID := ldStringVal(v)
		if childID == "" {
			continue
		}
		block := blockByID[childID]
		if block == nil {
			continue
		}
		if nodeHasType(block, dcsSectionTypeIRI) {
			sec := sectionData{
				Heading: strings.TrimSpace(ldFirstString(block, dcsTitleIRI)),
				Clauses: []clauseData{},
			}
			if secLayout, ok := layoutByID[childID]; ok {
				sec = walkSectionNode(sec, secLayout, layoutByID, blockByID)
			}
			sections = append(sections, sec)
		}
	}
	return sections
}

// walkSectionNode populates a sectionData's Clauses and Subsections from a layout node's children.
func walkSectionNode(sec sectionData, ln map[string]any, layoutByID map[string]map[string]any, blockByID map[string]map[string]any) sectionData {
	for _, v := range ldGetList(ln, dcsChildrenIRI) {
		childID := ldStringVal(v)
		if childID == "" {
			continue
		}
		block := blockByID[childID]
		if block == nil {
			continue
		}
		switch {
		case nodeHasType(block, dcsClauseTypeIRI) || nodeHasType(block, dcsTextBlockTypeIRI):
			sec.Clauses = append(sec.Clauses, parseNewClause(block))
		case nodeHasType(block, dcsSectionTypeIRI):
			sub := sectionData{
				Heading: strings.TrimSpace(ldFirstString(block, dcsTitleIRI)),
				Clauses: []clauseData{},
			}
			if subLayout, ok := layoutByID[childID]; ok {
				sub = walkSectionNode(sub, subLayout, layoutByID, blockByID)
			}
			sec.Subsections = append(sec.Subsections, sub)
		}
	}
	return sec
}

// parseNewClause extracts a clauseData from an expanded Clause node.
func parseNewClause(node map[string]any) clauseData {
	clause := clauseData{Segments: []clauseSegment{}}
	for _, c := range ldGetList(node, dcsContentIRI) {
		clause.Segments = append(clause.Segments, parseExpandedSegment(c))
	}
	return clause
}

// nodeHasType reports whether the expanded node has the given RDF type IRI.
func nodeHasType(node map[string]any, typeIRI string) bool {
	if node == nil {
		return false
	}
	for _, t := range ldGet(node, "@type") {
		if s, ok := t.(string); ok && s == typeIRI {
			return true
		}
	}
	return false
}

// nodeIsRoot reports whether the expanded LayoutNode has isRoot: true.
func nodeIsRoot(node map[string]any) bool {
	for _, v := range ldGet(node, dcsIsRootIRI) {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if b, ok := m["@value"].(bool); ok && b {
			return true
		}
	}
	return false
}

func findExpandedRootNode(expanded []any, rootID string) (map[string]any, bool) {
	for _, item := range expanded {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if rootID != "" {
			if id, _ := node["@id"].(string); id == rootID {
				return node, true
			}
		}
	}
	for _, item := range expanded {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if nodeHasType(node, dcsOntologyIRI+"ContractTemplate") {
			return node, true
		}
	}
	for _, item := range expanded {
		node, ok := item.(map[string]any)
		if ok {
			return node, true
		}
	}
	return nil, false
}

// parseExpandedSegment dispatches an expanded JSON-LD segment item to the
// appropriate clauseSegment type.
func parseExpandedSegment(item any) clauseSegment {
	node, ok := item.(map[string]any)
	if !ok {
		return clauseSegment{Type: "prose"}
	}

	if val, ok := node["@value"].(string); ok {
		if typ, ok := node["@type"].(string); ok && typ != "" {
			seg := clauseSegment{Type: "typed-value", Value: val, Datatype: typ}
			if u := ldFirstString(node, modelUnitCodeIRIs...); u != "" {
				seg.Unit = u
			}
			return seg
		}
		return clauseSegment{Type: "prose", Text: val}
	}

	if id, ok := node["@id"].(string); ok {
		text := id
		if n := ldFirstString(node, modelLinkNameIRIs...); n != "" {
			text = n
		}
		return clauseSegment{Type: "ontology-link", Text: text, Ref: id}
	}

	urlArr := ldGetAny(node, modelExternalURLIRIs...)
	if len(urlArr) > 0 {
		href := ""
		if v, ok := urlArr[0].(map[string]any); ok {
			if s, ok := v["@value"].(string); ok {
				href = s
			} else if id, ok := v["@id"].(string); ok {
				href = id
			}
		}
		text := href
		if n := ldFirstString(node, modelLinkNameIRIs...); n != "" {
			text = n
		}
		return clauseSegment{Type: "external-link", Text: text, Href: href}
	}

	return clauseSegment{Type: "prose"}
}

// ---- JSON-LD expanded-form navigation helpers --------------------------------

func ldGet(node map[string]any, iri string) []any {
	arr, _ := node[iri].([]any)
	return arr
}

// ldGetList is like ldGet but also unwraps a JSON-LD @list envelope.
// When a property is declared with @container:@list in the context, or when
// the payload uses explicit {"@list":[...]} syntax, JSON-LD expansion wraps
// the items in a single {"@list":[...]} map. ldGetList unwraps that so callers
// always receive the actual item slice regardless of which notation was used.
func ldGetList(node map[string]any, iri string) []any {
	arr := ldGet(node, iri)
	if len(arr) == 1 {
		if m, ok := arr[0].(map[string]any); ok {
			if list, ok := m["@list"].([]any); ok {
				return list
			}
		}
	}
	return arr
}

func ldGetAny(node map[string]any, iris ...string) []any {
	for _, iri := range iris {
		if iri == "" {
			continue
		}
		if arr := ldGet(node, iri); len(arr) > 0 {
			return arr
		}
	}
	return nil
}

func ldStringVal(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m["@value"].(string)
	return s
}

func ldFirstString(node map[string]any, iris ...string) string {
	for _, iri := range iris {
		if iri == "" {
			continue
		}
		for _, v := range ldGet(node, iri) {
			if s := ldStringVal(v); s != "" {
				return s
			}
		}
	}
	return ""
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

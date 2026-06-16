package compiler

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// extractDocumentModel builds a documentModel from the JSON-LD expanded form
// (where every property name is a full IRI) and the raw @context map.
//
// Using the expanded form rather than the raw JSON means property lookup is
// IRI-based: dcterms:title, dcs-pdf-core:sections, and schema:name are all
// resolved to their full IRIs before extraction, so the compiler is not
// sensitive to the compact key spellings in the submitted payload.
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

	// Build namespace map from raw @context (for ontology fetching and compact display).
	for prefix, uri := range rawCtx {
		if uriStr, ok := uri.(string); ok && !strings.HasPrefix(prefix, "@") {
			model.NamespaceMap[prefix] = uriStr
		}
	}

	if len(expanded) == 0 {
		return model, fmt.Errorf("dcs-pdf-core:title is required but the payload expanded to an empty graph")
	}
	root, ok := findExpandedRootNode(expanded, rootID)
	if !ok {
		return model, fmt.Errorf("dcs-pdf-core:title is required but no root node was found in the payload")
	}

	// Title — only disambiguated title IRIs are considered. Missing title is an error.
	t := ldFirstString(root, modelTitleIRIs...)
	if t == "" {
		return model, fmt.Errorf("dcs-pdf-core:title is required but was not found in the payload")
	}
	model.Title = t

	for _, item := range ldGetAny(root, modelSectionsIRIs...) {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, isVal := node["@value"]; isVal {
			continue
		}
		model.Sections = append(model.Sections, parseExpandedSection(node))
	}

	// Top-level clauses (single unnamed section).
	if len(model.Sections) == 0 {
		if clauseItems := ldGetAny(root, modelClausesIRIs...); len(clauseItems) > 0 {
			sec := sectionData{}
			for _, item := range clauseItems {
				sec.Clauses = append(sec.Clauses, parseExpandedClause(item))
			}
			model.Sections = append(model.Sections, sec)
		}
	}

	// Signature fields.
	for _, item := range ldGetAny(root, modelSignatureFieldsIRIs...) {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// A plain string in the array expands to {"@value": "..."}.
		if val, ok := node["@value"].(string); ok {
			if name := strings.TrimSpace(val); name != "" {
				model.SignatureFields = append(model.SignatureFields, sigFieldDef{Name: name, Label: name})
			}
			continue
		}
		sig := parseExpandedSignatureField(node)
		if sig.Name != "" {
			model.SignatureFields = append(model.SignatureFields, sig)
		}
	}

	// Compact ontology-link segment text that still holds the raw full IRI
	// (i.e. no explicit schema:name was supplied in the payload). Do this after
	// the namespace map is built so compactIRI has the prefix bindings available.
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

	// Glossary: collect ontology-link IRIs from section content in depth-first encounter order.
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
		if len(ldGetAny(node, modelSectionsIRIs...)) > 0 ||
			len(ldGetAny(node, modelSignatureFieldsIRIs...)) > 0 ||
			len(ldGetAny(node, modelTitleIRIs...)) > 0 {
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

// parseExpandedSection extracts a sectionData from an expanded JSON-LD node,
// recursing into subsections to arbitrary depth.
func parseExpandedSection(node map[string]any) sectionData {
	sec := sectionData{}
	sec.Heading = strings.TrimSpace(ldFirstString(node, modelHeadingIRIs...))
	for _, item := range ldGetAny(node, modelClausesIRIs...) {
		sec.Clauses = append(sec.Clauses, parseExpandedClause(item))
	}
	for _, item := range ldGetAny(node, modelSubsectionsIRIs...) {
		subNode, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, isVal := subNode["@value"]; isVal {
			continue
		}
		sec.Subsections = append(sec.Subsections, parseExpandedSection(subNode))
	}
	return sec
}

// parseExpandedClause extracts a clauseData from a single expanded clause item,
// which may be a plain string value object or a node with a "content" array.
func parseExpandedClause(item any) clauseData {
	clause := clauseData{Segments: []clauseSegment{}}
	node, ok := item.(map[string]any)
	if !ok {
		return clause
	}
	// Value object: plain string clause.
	if val, ok := node["@value"].(string); ok {
		clause.Segments = append(clause.Segments, clauseSegment{Type: "prose", Text: val})
		return clause
	}
	// Node with a content array of mixed prose/link/value segments.
	for _, c := range ldGetAny(node, modelContentIRIs...) {
		clause.Segments = append(clause.Segments, parseExpandedSegment(c))
	}
	return clause
}

// parseExpandedSegment dispatches an expanded JSON-LD segment item to the
// appropriate clauseSegment type: prose (plain string), typed-value (@value+@type),
// ontology-link (@id node), or external-link (schema:url node).
func parseExpandedSegment(item any) clauseSegment {
	node, ok := item.(map[string]any)
	if !ok {
		return clauseSegment{Type: "prose"}
	}

	// Value object: {"@value": "...", "@type": "..."} or plain {"@value": "..."}.
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

	// Node with @id: ontology link.
	if id, ok := node["@id"].(string); ok {
		text := id
		if n := ldFirstString(node, modelLinkNameIRIs...); n != "" {
			text = n
		}
		return clauseSegment{Type: "ontology-link", Text: text, Ref: id}
	}

	// Node with a disambiguated URL IRI: external link.
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

// parseExpandedSignatureField extracts a sigFieldDef from an expanded JSON-LD node.
func parseExpandedSignatureField(node map[string]any) sigFieldDef {
	sig := sigFieldDef{}
	name := ldFirstString(node, modelSigNameIRIs...)
	sig.Name = strings.TrimSpace(name)
	label := ldFirstString(node, modelSigLabelIRIs...)
	sig.Label = strings.TrimSpace(label)
	if sig.Label == "" {
		sig.Label = sig.Name
	}
	return sig
}

// ---- JSON-LD expanded-form navigation helpers --------------------------------

// ldGet returns the value array of a property given its full IRI.
// In expanded JSON-LD all property values are arrays.
func ldGet(node map[string]any, iri string) []any {
	arr, _ := node[iri].([]any)
	return arr
}

// ldGetAny returns the first non-empty array for the provided candidate IRIs.
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

// ldStringVal extracts the plain string value from an expanded value node.
func ldStringVal(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m["@value"].(string)
	return s
}

// ldFirstString tries each full IRI in order and returns the first string value.
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

var dcsCoreIRI string

var (
	modelTitleIRIs           []string
	modelSectionsIRIs        []string
	modelHeadingIRIs         []string
	modelClausesIRIs         []string
	modelContentIRIs         []string
	modelSubsectionsIRIs     []string
	modelSignatureFieldsIRIs []string
	modelSigNameIRIs         []string
	modelSigLabelIRIs        []string

	modelUnitCodeIRIs    = []string{"https://schema.org/unitCode", "http://schema.org/unitCode"}
	modelLinkNameIRIs    = []string{"https://schema.org/name", "http://schema.org/name"}
	modelExternalURLIRIs = []string{"https://schema.org/url", "http://schema.org/url"}
)

func init() {
	initOntologyIRI(os.Getenv(envOntologyBaseURL))
}

// InitOntologyIRI sets dcsCoreIRI and all derived model IRI slices from baseURL.
// An empty baseURL defaults to http://127.0.0.1:8080.
// Call this from main() after loading .env so the runtime URL overrides the
// value set by init().
func InitOntologyIRI(baseURL string) {
	initOntologyIRI(baseURL)
}

func initOntologyIRI(baseURL string) {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	dcsCoreIRI = baseURL + "/ontology/dcs-pdf-core#"
	modelTitleIRIs = []string{dcsCoreIRI + "title"}
	modelSectionsIRIs = []string{dcsCoreIRI + "sections"}
	modelHeadingIRIs = []string{dcsCoreIRI + "heading"}
	modelClausesIRIs = []string{dcsCoreIRI + "clauses"}
	modelContentIRIs = []string{dcsCoreIRI + "content"}
	modelSubsectionsIRIs = []string{dcsCoreIRI + "subsections"}
	modelSignatureFieldsIRIs = []string{dcsCoreIRI + "signatureFields"}
	modelSigNameIRIs = []string{dcsCoreIRI + "name", "https://schema.org/name", "http://schema.org/name"}
	modelSigLabelIRIs = []string{dcsCoreIRI + "label", "https://schema.org/name", "http://schema.org/name"}
}

// compactIRI converts a full IRI to a prefix:localName form using the namespace map,
// or returns the IRI itself when no matching prefix is registered.
// Prefixes are checked in sorted order to guarantee deterministic output when
// multiple prefixes map to the same base URI.
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


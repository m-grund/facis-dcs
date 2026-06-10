package compiler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func extractDocumentModel(expanded []any, rootID string, rawCtx map[string]any, canonical []byte, hashHex string) documentModel {
	model := documentModel{
		Title:           "Deterministic Semantic Ledger",
		Sections:        []sectionData{},
		SignatureFields: []sigFieldDef{},
		Glossary:        []glossaryTerm{},
		NamespaceMap:    make(map[string]string),
		CanonicalJSON:   canonical,
		PayloadHash:     hashHex,
		FileID:          hashHex[:32],
	}

	// Build namespace map from raw @context (for ontology fetching and compact display).
	for prefix, uri := range rawCtx {
		if uriStr, ok := uri.(string); ok && !strings.HasPrefix(prefix, "@") {
			model.NamespaceMap[prefix] = uriStr
		}
	}

	if len(expanded) == 0 {
		return model
	}
	root, ok := findExpandedRootNode(expanded, rootID)
	if !ok {
		return model
	}

	// Title — only disambiguated title IRIs are considered.
	if t := ldFirstString(root, modelTitleIRIs...); t != "" {
		model.Title = t
	}

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

	// Glossary: fetch ontology term definitions, then match against ontology-link
	// references collected from section content (depth-first, encounter order).
	allTerms := ontologyTerms(rawCtx)
	referencedIRIs := make(map[string]struct{})
	var refIRIOrder []string
	var collectSectionRefs func(sections []sectionData)
	collectSectionRefs = func(sections []sectionData) {
		for _, section := range sections {
			for _, clause := range section.Clauses {
				for _, seg := range clause.Segments {
					if seg.Type == "ontology-link" && seg.Ref != "" {
						if _, seen := referencedIRIs[seg.Ref]; !seen {
							refIRIOrder = append(refIRIOrder, seg.Ref)
							referencedIRIs[seg.Ref] = struct{}{}
						}
					}
				}
			}
			collectSectionRefs(section.Subsections)
		}
	}
	collectSectionRefs(model.Sections)

	seen := map[string]struct{}{}
	for _, refIRI := range refIRIOrder {
		if _, exists := seen[refIRI]; exists {
			continue
		}
		seen[refIRI] = struct{}{}
		found := false
		for _, term := range allTerms {
			if term.TermURI == refIRI {
				model.Glossary = append(model.Glossary, term)
				found = true
				break
			}
		}
		if !found {
			// Compact the IRI to a prefix:localName form for display if possible.
			display := compactIRI(refIRI, model.NamespaceMap)
			model.Glossary = append(model.Glossary, glossaryTerm{
				Term:    display,
				TermURI: refIRI,
			})
		}
	}

	return model
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

// ---- Ontology term fetching --------------------------------------------------

// ontologyTerms resolves each namespace URI declared in the @context by
// performing a fresh HTTP GET (Cache-Control: no-cache) against the URI and
// parsing the response as a JSON-LD expanded array. Terms are extracted from
// rdfs:comment or skos:definition, falling back to rdfs:label.
// This runs on every compilation — no caching — so the self-hosted ontology
// at http://127.0.0.1:8080/ontology/dcs-pdf-core is always fetched live.
func ontologyTerms(ctx map[string]any) []glossaryTerm {
	client := &http.Client{}
	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	terms := make([]glossaryTerm, 0)
	for _, prefix := range keys {
		uri, ok := ctx[prefix].(string)
		if !ok || strings.HasPrefix(prefix, "@") {
			continue
		}
		fetchURL := strings.TrimRight(uri, "#/")
		req, err := http.NewRequest(http.MethodGet, fetchURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Accept", "application/ld+json, application/json;q=0.9, text/turtle;q=0.5")
		req.Header.Set("Cache-Control", "no-cache")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		var doc any
		if err := json.Unmarshal(body, &doc); err != nil {
			continue
		}
		terms = append(terms, extractOntologyTerms(prefix, uri, doc)...)
	}
	return terms
}

// extractOntologyTerms walks a fetched ontology document (already parsed by
// the document loader into a Go value) and collects rdfs:label +
// rdfs:comment pairs for any entity whose @id starts with the namespace URI.
func extractOntologyTerms(prefix, nsURI string, doc any) []glossaryTerm {
	var nodes []any
	switch v := doc.(type) {
	case []any:
		nodes = v
	case map[string]any:
		nodes = []any{v}
	default:
		return nil
	}

	terms := make([]glossaryTerm, 0)
	seen := map[string]struct{}{}
	for _, node := range nodes {
		entry, ok := node.(map[string]any)
		if !ok {
			continue
		}
		id, _ := entry["@id"].(string)
		if !strings.HasPrefix(id, nsURI) {
			continue
		}
		localName := strings.TrimPrefix(id, nsURI)
		if localName == "" {
			continue
		}
		prefixed := prefix + ":" + localName
		if _, dup := seen[prefixed]; dup {
			continue
		}
		seen[prefixed] = struct{}{}
		definition := rdfsValue(entry, "http://www.w3.org/2000/01/rdf-schema#comment")
		if definition == "" {
			definition = rdfsValue(entry, "http://www.w3.org/2004/02/skos/core#definition")
		}
		if definition == "" {
			definition = rdfsValue(entry, "http://www.w3.org/2000/01/rdf-schema#label")
		}
		if definition == "" {
			definition = fmt.Sprintf("<%s>", id)
		}
		terms = append(terms, glossaryTerm{Term: prefixed, Definition: definition, TermURI: id})
	}
	return terms
}

// rdfsValue extracts the first plain-string value of an RDF property from
// an already-expanded JSON-LD node map.
func rdfsValue(node map[string]any, prop string) string {
	val, ok := node[prop]
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		for _, item := range v {
			switch s := item.(type) {
			case string:
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			case map[string]any:
				if t, ok := s["@value"].(string); ok && strings.TrimSpace(t) != "" {
					return strings.TrimSpace(t)
				}
			}
		}
	}
	return ""
}

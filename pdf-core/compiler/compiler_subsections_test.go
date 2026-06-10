package compiler

import (
	"bytes"
	"regexp"
	"testing"
)

// subsectionDoc builds a documentModel with one top-level section containing
// a subsection, for use in subsection rendering tests.
func subsectionDoc() documentModel {
	return sectionDoc([]sectionData{
		{
			Heading: "1. Top-Level",
			Clauses: []clauseData{
				{Segments: []clauseSegment{{Type: "prose", Text: "Top-level clause."}}},
			},
			Subsections: []sectionData{
				{
					Heading: "1.1 Subsection",
					Clauses: []clauseData{
						{Segments: []clauseSegment{{Type: "prose", Text: "Subsection clause."}}},
					},
				},
			},
		},
	})
}

// TestSubsectionHeadingAppearsInPDF verifies that a subsection heading appears
// in the compiled PDF content streams.
func TestSubsectionHeadingAppearsInPDF(t *testing.T) {
	pdf := renderPDF(subsectionDoc())
	if !bytes.Contains(concatBTBlocks(pdf), []byte("1.1 Subsection")) {
		t.Error("subsection heading not found in PDF content streams")
	}
}

// TestSubsectionHeadingIsH2InContentStream verifies that a depth-1 subsection
// heading is tagged /H2 in the PDF content stream.
func TestSubsectionHeadingIsH2InContentStream(t *testing.T) {
	pdf := renderPDF(subsectionDoc())
	if !bytes.Contains(pdf, []byte("/H2 ")) {
		t.Error("depth-1 subsection heading must produce /H2 tag in content stream")
	}
}

// TestSubsubsectionHeadingIsH3InContentStream verifies that a depth-2 heading
// is tagged /H3 in the PDF content stream.
func TestSubsubsectionHeadingIsH3InContentStream(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{
			Heading: "1. Top",
			Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "Top."}}}},
			Subsections: []sectionData{
				{
					Heading: "1.1 Sub",
					Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "Sub."}}}},
					Subsections: []sectionData{
						{
							Heading: "1.1.1 SubSub",
							Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "SubSub."}}}},
						},
					},
				},
			},
		},
	})
	pdf := renderPDF(doc)
	if !bytes.Contains(pdf, []byte("/H3 ")) {
		t.Error("depth-2 subsection heading must produce /H3 tag in content stream")
	}
}

// TestSubsectionIndentedInLayout verifies that a subsection heading has a larger
// X offset than its parent section heading in the layout output.
func TestSubsectionIndentedInLayout(t *testing.T) {
	doc := subsectionDoc()
	pages := layoutDocumentPages(doc)

	parentX, subX := -1.0, -1.0
	for _, page := range pages {
		for _, line := range page.Lines {
			if line.Kind == "section-heading" && line.Text == "1. Top-Level" {
				parentX = line.X
			}
			if line.Kind == "subsection-heading" && line.Text == "1.1 Subsection" {
				subX = line.X
			}
		}
	}
	if parentX < 0 {
		t.Fatal("parent section heading not found in layout")
	}
	if subX < 0 {
		t.Fatal("subsection heading not found in layout")
	}
	if subX <= parentX {
		t.Errorf("subsection X (%.2f) must be greater than parent section X (%.2f) — no indentation", subX, parentX)
	}
}

// TestSubsectionStructTreeHasH2 verifies that the PDF structure tree contains
// an /S /H2 element for a depth-1 subsection heading.
func TestSubsectionStructTreeHasH2(t *testing.T) {
	pdf := renderPDF(subsectionDoc())
	if !bytes.Contains(pdf, []byte("/S /H2")) {
		t.Error("PDF struct tree must contain /S /H2 for depth-1 subsection")
	}
}

// TestSubsectionStructTreeDocumentKidsExcludesSubsects verifies that the
// /S /Document struct element's /K array contains only top-level sections —
// not subsections.  Subsections must appear as children of their parent /Sect
// element, not as direct children of /Document.
//
// With 1 top-level section and 1 subsection the /Document /K must have exactly
// 3 refs: [title, meta, topSect].
func TestSubsectionStructTreeDocumentKidsExcludesSubsects(t *testing.T) {
	pdf := renderPDF(subsectionDoc())
	re := regexp.MustCompile(`/S /Document[^>]*/K \[([^\]]+)\]`)
	m := re.FindSubmatch(pdf)
	if m == nil {
		t.Fatal("/S /Document struct element not found")
	}
	refs := regexp.MustCompile(`\d+ 0 R`).FindAllString(string(m[1]), -1)
	if len(refs) != 3 {
		t.Errorf("/Document struct elem /K must have 3 children [title meta topSect], got %d: %v", len(refs), refs)
	}
}

// TestSubsectionSectContainsChildSectRef verifies that the parent section's
// /Sect struct element has the subsection /Sect as one of its /K children.
// With 1 top-level section that has 1 clause and 1 subsection, the parent
// /Sect's /K must have at least 3 entries: [heading, para, subsectSect].
func TestSubsectionSectContainsChildSectRef(t *testing.T) {
	pdf := renderPDF(subsectionDoc())
	re := regexp.MustCompile(`/S /Sect[^>]*/K \[([^\]]+)\]`)
	matches := re.FindAllSubmatch(pdf, -1)
	if len(matches) < 2 {
		t.Fatalf("expected 2+ /S /Sect elements, got %d", len(matches))
	}
	refsRe := regexp.MustCompile(`\d+ 0 R`)
	hasParentSect := false
	for _, m := range matches {
		if len(refsRe.FindAllString(string(m[1]), -1)) >= 3 {
			hasParentSect = true
		}
	}
	if !hasParentSect {
		t.Error("no /S /Sect has 3+ /K children — subsection /Sect must be nested inside parent /Sect /K")
	}
}

// TestSubsectionNestedInOutline verifies that the PDF outline contains an entry
// for the subsection heading.
func TestSubsectionNestedInOutline(t *testing.T) {
	pdf := renderPDF(subsectionDoc())
	if !bytes.Contains(pdf, []byte("/Title (1.1 Subsection)")) {
		t.Error("PDF outline must contain an entry for the subsection heading")
	}
}

// TestSubsectionDeterministic verifies that a document with nested subsections
// produces identical PDFs across two compilations.
func TestSubsectionDeterministic(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{
			Heading: "Top",
			Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "Top clause."}}}},
			Subsections: []sectionData{
				{
					Heading: "Sub-A",
					Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "Sub-A."}}}},
					Subsections: []sectionData{
						{Heading: "SubSub", Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "SubSub."}}}}},
					},
				},
				{Heading: "Sub-B", Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "Sub-B."}}}}},
			},
		},
	})
	pdf1 := renderPDF(doc)
	pdf2 := renderPDF(doc)
	if !bytes.Equal(pdf1, pdf2) {
		t.Error("subsection document must produce byte-for-byte identical PDFs across compilations")
	}
}

// TestSubsectionDepthFirstOrder verifies that headings appear in depth-first
// order in the page content streams: Top → Sub-A → SubSub → Sub-B.
func TestSubsectionDepthFirstOrder(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{
			Heading: "Parent-Section",
			Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "Parent."}}}},
			Subsections: []sectionData{
				{
					Heading: "SubA-Section",
					Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "SubA."}}}},
					Subsections: []sectionData{
						{Heading: "SubSubA-Section", Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "SubSubA."}}}}},
					},
				},
				{Heading: "SubB-Section", Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "SubB."}}}}},
			},
		},
	})
	order := []string{"Parent-Section", "SubA-Section", "SubSubA-Section", "SubB-Section"}
	content := concatBTBlocks(renderPDF(doc))
	positions := make([]int, len(order))
	for i, h := range order {
		pos := bytes.Index(content, []byte(h))
		if pos < 0 {
			t.Fatalf("heading %q not found in page content streams", h)
		}
		positions[i] = pos
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			t.Errorf("heading %q (offset %d) must follow %q (offset %d) in depth-first order",
				order[i], positions[i], order[i-1], positions[i-1])
		}
	}
}

// TestSubsectionFromPayload verifies that a JSON-LD payload with nested
// subsections compiles without error and all headings appear in the PDF.
func TestSubsectionFromPayload(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:subsection-test",
		"@type": "dcs-pdf-core:Document",
		"title": "Subsection Test Document",
		"sections": [
			{
				"@type": "dcs-pdf-core:Section",
				"heading": "1. Main Section",
				"clauses": ["Main clause."],
				"subsections": [
					{
						"@type": "dcs-pdf-core:Section",
						"heading": "1.1 First Sub",
						"clauses": ["Sub clause one."],
						"subsections": [
							{
								"@type": "dcs-pdf-core:Section",
								"heading": "1.1.1 Deep Sub",
								"clauses": ["Deep sub clause."]
							}
						]
					},
					{
						"@type": "dcs-pdf-core:Section",
						"heading": "1.2 Second Sub",
						"clauses": ["Sub clause two."]
					}
				]
			}
		]
	}`)
	doc := mustExtractFromPayload(t, payload)
	if len(doc.Sections) == 0 {
		t.Fatal("no sections extracted from payload")
	}
	if len(doc.Sections[0].Subsections) != 2 {
		t.Fatalf("expected 2 subsections, got %d", len(doc.Sections[0].Subsections))
	}
	if len(doc.Sections[0].Subsections[0].Subsections) != 1 {
		t.Fatalf("expected 1 sub-subsection, got %d", len(doc.Sections[0].Subsections[0].Subsections))
	}

	pdf := renderPDF(doc)
	for _, heading := range []string{"1. Main Section", "1.1 First Sub", "1.1.1 Deep Sub", "1.2 Second Sub"} {
		if !bytes.Contains(concatBTBlocks(pdf), []byte(heading)) {
			t.Errorf("heading %q not found in compiled PDF", heading)
		}
	}
}

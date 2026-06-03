package builder

import (
	"bytes"
	"os"
	"testing"
	"time"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedTemplateInput is the canonical deterministic input for template conformance tests.
var fixedTemplateInput = TemplateInput{
	DID:            "did:example:tmpl",
	State:          "draft",
	Version:        1,
	Name:           "Test Template",
	Description:    "veraPDF conformance test for template builder",
	TemplateType:   "contract",
	DocumentNumber: "TMP-001",
	CreatedBy:      "test-user",
	CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	UpdatedAt:      time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	TemplateData:   []byte(`{"@context":"https://www.w3.org/2018/credentials/v1","type":"Template"}`),
}

// pdfa3Case parameterises PDF/A-3 conformance checks over both builder outputs.
type pdfa3Case struct {
	name    string
	outPath string
	build   func(t *testing.T) []byte
}

var pdfa3Cases = []pdfa3Case{
	{
		name:    "contract",
		outPath: "/tmp/test_contract.pdf",
		build: func(t *testing.T) []byte {
			t.Helper()
			pdf, err := BuildContract(fixedInput)
			require.NoError(t, err)
			return pdf
		},
	},
	{
		name:    "template",
		outPath: "/tmp/test_template.pdf",
		build: func(t *testing.T) []byte {
			t.Helper()
			pdf, err := BuildTemplate(fixedTemplateInput)
			require.NoError(t, err)
			return pdf
		},
	},
}

// TestExportPDFsForVeraPDF writes both builder outputs to /tmp for manual veraPDF inspection.
func TestExportPDFsForVeraPDF(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			require.NoError(t, os.WriteFile(tc.outPath, pdf, 0644))
			t.Logf("wrote %s", tc.outPath)
		})
	}
}

func TestPDFA3_BinaryHeader(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			firstNL := bytes.IndexByte(pdf, '\n')
			require.Greater(t, firstNL, -1)
			secondLineEnd := bytes.IndexByte(pdf[firstNL+1:], '\n')
			require.Greater(t, secondLineEnd, -1)
			second := pdf[firstNL+1 : firstNL+1+secondLineEnd]
			require.GreaterOrEqual(t, len(second), 5)
			assert.Equal(t, byte('%'), second[0])
			assert.Greater(t, int(second[1]), 127)
			assert.Greater(t, int(second[2]), 127)
			assert.Greater(t, int(second[3]), 127)
			assert.Greater(t, int(second[4]), 127)
		})
	}
}

func TestPDFA3_TrailerID(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			trailIdx := bytes.LastIndex(pdf, []byte("trailer"))
			require.Greater(t, trailIdx, -1)
			assert.Contains(t, string(pdf[trailIdx:]), "/ID [<")
		})
	}
}

func TestPDFA3_OutputIntent(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			assert.Contains(t, string(pdf), "/OutputIntents")
			assert.Contains(t, string(pdf), "/GTS_PDFA1")
		})
	}
}

func TestPDFA3_MetadataLinked(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			assert.Contains(t, string(pdf), "/Type /Metadata")
			assert.True(t, hasCatalogWithMetadataLink(pdf), "catalog must contain /Metadata link")
			assert.Contains(t, string(pdf), "<pdfaid:conformance>A</pdfaid:conformance>")
		})
	}
}

func TestPDFA3_TaggingMarkers(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			assert.Contains(t, string(pdf), "/StructTreeRoot")
			assert.Contains(t, string(pdf), "/MarkInfo")
			assert.Contains(t, string(pdf), "/Marked true")
			assert.Contains(t, string(pdf), "/Lang")
			assert.Contains(t, string(pdf), "/ParentTree")
			assert.Contains(t, string(pdf), "/MCID 0")
		})
	}
}

func TestPDFA3_AFRelationship(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			assert.Contains(t, string(pdf), "/AFRelationship /Data")
		})
	}
}

func TestPDFA3_EmbeddedFileMIME(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			assert.Contains(t, string(pdf), "/Subtype /application#2Fjson")
		})
	}
}

func TestPDFA3_EmbeddedFileModDate(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			assert.Contains(t, string(pdf), "/ModDate (D:19700101000000)")
		})
	}
}

func TestPDFA3_Determinism(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf1 := tc.build(t)
			pdf2 := tc.build(t)
			assert.Equal(t, pdf1, pdf2)
		})
	}
}

func TestPDFA3_PdfcpuParseable(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			_, err := pdfapi.ReadContext(bytes.NewReader(pdf), conf)
			require.NoError(t, err)
		})
	}
}

func TestPDFA3_XRefConsistency(t *testing.T) {
	for _, tc := range pdfa3Cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pdf := tc.build(t)
			start := extractLastStartXRefLocal(pdf)
			require.Greater(t, start, int64(0))
			require.Less(t, int(start), len(pdf))
			assert.True(t, bytes.HasPrefix(pdf[start:], []byte("xref")), "startxref must point to xref keyword")
		})
	}
}

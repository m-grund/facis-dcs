package builder

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
)

// TemplateInput holds the data needed to render a template PDF.
type TemplateInput struct {
	DID            string
	State          string
	Version        int
	Name           string
	Description    string
	TemplateType   string
	DocumentNumber string
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// TemplateData is the raw JSON-LD bytes stored in the DB.
	TemplateData []byte
}

// BuildTemplate renders a TemplateInput to a deterministic PDF/A-3 []byte.
// TemplateData bytes are embedded as "contract.jsonld" for MR/HR cross-validation.
func BuildTemplate(in TemplateInput) ([]byte, error) {
	f := newBase()
	registerFooter(f)

	title := in.Name
	if title == "" {
		title = "Contract Template"
	}

	f.SetXmpMetadata(xmpMetadata(title, in.DID))
	f.AddPage()

	renderHeader(f, title, in.DID, "")

	if len(in.TemplateData) > 0 {
		renderSection(f, "Template Terms")
		renderContractData(f, in.TemplateData)
	}

	if len(in.TemplateData) > 0 {
		f.SetAttachments([]fpdf.Attachment{
			{
				Content:     in.TemplateData,
				Filename:    "template.jsonld",
				Description: "Machine-readable JSON-LD source for this template",
			},
		})
	}

	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return nil, fmt.Errorf("render template PDF: %w", err)
	}
	return buf.Bytes(), nil
}

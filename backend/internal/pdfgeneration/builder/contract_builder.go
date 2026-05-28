package builder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/go-pdf/fpdf"
)

// ContractInput holds the data needed to render a contract PDF.
type ContractInput struct {
	DID         string
	State       string
	Version     int
	Name        string
	Description string
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// ContractData is the raw JSON-LD bytes stored in the DB. It is embedded
	// verbatim in the PDF so the human-readable and machine-readable forms stay
	// in sync (DCS-FR-CWE-04).
	ContractData []byte
}

// BuildContract renders a ContractInput to a deterministic PDF/A-3 []byte.
// The ContractData bytes are ALSO embedded as an EmbeddedFile attachment named
// "contract.jsonld" so callers can later extract, re-render, and compare hashes
// to verify MR/HR consistency (DCS-FR-CWE-04, DCS-FR-CWE-05).
func BuildContract(in ContractInput) ([]byte, error) {
	f := newBase()
	registerFooter(f)

	title := in.Name
	if title == "" {
		title = "Contract"
	}

	f.SetXmpMetadata(xmpMetadata(title, in.DID))
	f.AddPage()

	renderHeader(f, title, in.DID, in.State)

	renderSection(f, "Contract Details")
	renderKV(f, "DID", in.DID)
	renderKV(f, "Version", fmt.Sprintf("%d", in.Version))
	renderKV(f, "State", in.State)
	if in.Description != "" {
		renderKV(f, "Description", in.Description)
	}
	renderKV(f, "Created by", in.CreatedBy)
	renderKV(f, "Created at", in.CreatedAt.UTC().Format(time.RFC3339))
	renderKV(f, "Updated at", in.UpdatedAt.UTC().Format(time.RFC3339))

	if len(in.ContractData) > 0 {
		renderSection(f, "Contract Terms (JSON-LD)")
		renderJSONLD(f, in.ContractData)
	}

	// Embed the JSON-LD as an attachment so it can be extracted for MR/HR
	// cross-validation (DCS-FR-CWE-04).
	if len(in.ContractData) > 0 {
		f.SetAttachments([]fpdf.Attachment{
			{
				Content:     in.ContractData,
				Filename:    "contract.jsonld",
				Description: "Machine-readable JSON-LD source for this contract",
			},
		})
	}

	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return nil, fmt.Errorf("render contract PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// renderJSONLD pretty-prints top-level JSON fields into the PDF.
// Keys are sorted for deterministic rendering.
func renderJSONLD(f *fpdf.Fpdf, raw []byte) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		// Fall back to raw display if parsing fails.
		f.SetFont(fontFamily, fontRegular, sizeSmall)
		f.MultiCell(bodyWidth, lineHeight, string(raw), "", "L", false)
		return
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		var val any
		if err := json.Unmarshal(m[k], &val); err != nil {
			continue
		}
		rendered := renderValue(val, 0)
		renderKV(f, k, rendered)
	}
}

func renderValue(v any, depth int) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return ""
	case map[string]any:
		b, _ := json.MarshalIndent(t, "", "  ")
		return string(b)
	case []any:
		b, _ := json.MarshalIndent(t, "", "  ")
		return string(b)
	default:
		return fmt.Sprintf("%v", t)
	}
}

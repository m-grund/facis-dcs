package builder

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/go-pdf/fpdf"
)

const (
	pageWidth    = 210.0 // A4 mm
	pageHeight   = 297.0 // A4 mm
	marginLeft   = 20.0
	marginRight  = 20.0
	marginTop    = 25.0
	marginBottom = 20.0
	bodyWidth    = pageWidth - marginLeft - marginRight

	fontFamily  = "NotoSans"
	fontRegular = ""
	fontBold    = "B"

	sizeTitle   = 16.0
	sizeHeading = 12.0
	sizeBody    = 10.0
	sizeSmall   = 8.0

	lineHeight = 5.5

	// producer is fixed to keep PDF bytes deterministic across builds.
	producer = "DCS PDF Generation v1"

	// RendererVersion is the semver identifier for this renderer build. It is
	// embedded in every C2PA lifecycle assertion so that future verification can
	// select the exact renderer that produced a given PDF (required for the
	// bytewise-match check to remain valid after renderer upgrades).
	RendererVersion = "1.0.1"
)

// epochTime is used as a fixed creation/modification date so the PDF bytes are
// deterministic for the same JSON-LD input (DCS-FR-CWE-04).
// time.Time{} (year 1) is treated as zero by fpdf and replaced with time.Now(),
// so we use Unix epoch (1970-01-01) instead.
var epochTime = time.Unix(0, 0).UTC()

// newBase returns an initialised Fpdf with NotoSans loaded, fixed metadata,
// and standard A4 margins. The caller should call AddPage() then render content.
func newBase() *fpdf.Fpdf {
	f := fpdf.NewCustom(&fpdf.InitType{
		UnitStr:    "mm",
		Size:       fpdf.SizeType{Wd: pageWidth, Ht: pageHeight},
		FontDirStr: "",
	})
	semanticReset(f)
	f.SetMargins(marginLeft, marginTop, marginRight)
	f.SetAutoPageBreak(true, marginBottom)
	f.SetProducer(producer, false)
	f.SetCreationDate(epochTime)
	f.SetModificationDate(epochTime)
	// SetCatalogSort ensures fonts, images and embedded files are serialized in
	// sorted key order rather than random Go map iteration order. Required for
	// byte-deterministic output (DCS-FR-CWE-04 / renderer trust model).
	f.SetCatalogSort(true)

	// Load vendored Noto Sans fonts (embedded via go:embed).
	f.AddUTF8FontFromBytes(fontFamily, fontRegular, notoSansRegular)
	f.AddUTF8FontFromBytes(fontFamily, fontBold, notoSansBold)

	return f
}

// renderHeader writes the standard document header onto the current page.
func renderHeader(f *fpdf.Fpdf, title, did, state string) {
	f.SetFont(fontFamily, fontBold, sizeTitle)
	f.SetTextColor(30, 30, 30)
	semanticWithTag(f, "H1", func() {
		f.CellFormat(bodyWidth, 10, title, "", 1, "L", false, 0, "")
	})

	f.SetFont(fontFamily, fontRegular, sizeSmall)
	f.SetTextColor(100, 100, 100)
	semanticWithTag(f, "P", func() {
		f.CellFormat(bodyWidth/2, 5, "DID: "+did, "", 0, "L", false, 0, "")
		f.CellFormat(bodyWidth/2, 5, "Status: "+state, "", 1, "R", false, 0, "")
	})
	f.Ln(3)

	// Horizontal rule
	f.SetDrawColor(200, 200, 200)
	x := f.GetX()
	y := f.GetY()
	f.Line(x, y, x+bodyWidth, y)
	f.Ln(4)
	f.SetTextColor(30, 30, 30)
}

// renderSection renders a bold section heading.
func renderSection(f *fpdf.Fpdf, heading string) {
	f.SetFont(fontFamily, fontBold, sizeHeading)
	f.Ln(3)
	semanticWithTag(f, "H2", func() {
		f.CellFormat(bodyWidth, 7, heading, "", 1, "L", false, 0, "")
	})
}

// renderKV renders a key-value row.
func renderKV(f *fpdf.Fpdf, key, value string) {
	semanticWithTag(f, "P", func() {
		f.SetFont(fontFamily, fontBold, sizeBody)
		f.CellFormat(50, lineHeight, key+":", "", 0, "L", false, 0, "")
		f.SetFont(fontFamily, fontRegular, sizeBody)
		f.MultiCell(bodyWidth-50, lineHeight, value, "", "L", false)
	})
}

// registerFooter adds page number footer to every page via SetFooterFunc.
func registerFooter(f *fpdf.Fpdf) {
	f.SetFooterFunc(func() {
		f.SetY(-15)
		f.SetFont(fontFamily, fontRegular, sizeSmall)
		f.SetTextColor(150, 150, 150)
		semanticArtifact(f, func() {
			f.CellFormat(0, 10, strconv.Itoa(f.PageNo()), "", 0, "C", false, 0, "")
		})
	})
}

// renderPDF serialises f and applies the PDF/A-3U post-processing pipeline.
// All builders must call this instead of f.Output directly so that
// fixPDFA3 is never bypassed by accident.
func renderPDF(f *fpdf.Fpdf) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return nil, fmt.Errorf("fpdf output: %w", err)
	}
	return fixPDFA3(buf.Bytes()), nil
}

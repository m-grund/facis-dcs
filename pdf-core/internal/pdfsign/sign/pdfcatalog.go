package sign

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/digitorus/pdf"
)

func (context *SignContext) createCatalog() ([]byte, error) {
	var catalog_buffer bytes.Buffer

	// Start the catalog object
	catalog_buffer.WriteString("<<\n")
	catalog_buffer.WriteString("  /Type /Catalog\n")

	// (Optional; PDF 1.4) The version of the PDF specification to which
	// the document conforms (for example, 1.4) if later than the version
	// specified in the file’s header (see 7.5.2, "File header"). If the header
	// specifies a later version, or if this entry is absent, the document
	// shall conform to the version specified in the header. This entry
	// enables a PDF processor to update the version using an incremental
	// update; see 7.5.6, "Incremental updates".
	// The value of this entry shall be a name object, not a number, and
	// therefore shall be preceded by a SOLIDUS (2Fh) character (/) when
	// written in the PDF file (for example, /1.4).
	//
	// If an incremental upgrade requires a version that is higher than specified by the document.
	// Ensure PDF version is at least 1.5 to support SigFlags in acroFormDict (1.4) and UF in the fileSpecDict (1.5)
	if v, err := strconv.ParseFloat(context.PDFReader.PDFVersion, 64); err == nil && v < 1.5 {
		catalog_buffer.WriteString("  /Version /1.5\n")
	}

	// Retrieve the root, its pointer and set the root string
	root := context.PDFReader.Trailer().Key("Root")
	rootPtr := root.GetPtr()
	context.CatalogData.RootString = strconv.Itoa(int(rootPtr.GetID())) + " " + strconv.Itoa(int(rootPtr.GetGen())) + " R"

	// Copy over existing catalog entries except for type and AcroForum
	for _, key := range root.Keys() {
		if key != "Type" && key != "AcroForm" {
			_, _ = fmt.Fprintf(&catalog_buffer, "  /%s ", key)
			context.serializeCatalogEntry(&catalog_buffer, rootPtr.GetID(), root.Key(key))
			catalog_buffer.WriteString("\n")
		}
	}

	// Start the AcroForm dictionary with /NeedAppearances
	catalog_buffer.WriteString("  /AcroForm <<\n")
	catalog_buffer.WriteString("    /Fields [")

	// Add existing signatures to the AcroForm dictionary
	for i, sig := range context.existingSignatures {
		if i > 0 {
			catalog_buffer.WriteString(" ")
		}
		catalog_buffer.WriteString(strconv.Itoa(int(sig.objectId)) + " 0 R")
	}

	// Add the newly created signature field to the AcroForm dictionary. When an
	// existing empty field was filled instead, it is already part of
	// existingSignatures above and must not be listed twice.
	if context.filledFieldObjectID == 0 {
		if len(context.existingSignatures) > 0 {
			catalog_buffer.WriteString(" ")
		}
		catalog_buffer.WriteString(strconv.Itoa(int(context.VisualSignData.objectId)) + " 0 R")
	}

	catalog_buffer.WriteString("]\n") // close Fields array

	// (Optional; deprecated in PDF 2.0) A flag specifying whether
	// to construct appearance streams and appearance
	// dictionaries for all widget annotations in the document (see
	// 12.7.4.3, "Variable text"). Default value: false. A PDF writer
	// shall include this key, with a value of true, if it has not
	// provided appearance streams for all visible widget
	// annotations present in the document.
	// if context.SignData.Visible {
	// 	catalog_buffer.WriteString(" /NeedAppearances true")
	// } else {
	// 	catalog_buffer.WriteString(" /NeedAppearances false")
	// }

	// Signature flags (Table 225)
	//
	// Bit position 1: SignaturesExist
	// If set, the document contains at least one signature field. This
	// flag allows an interactive PDF processor to enable user
	// interface items (such as menu items or push-buttons) related to
	// signature processing without having to scan the entire
	// document for the presence of signature fields.
	//
	// Bit position 2: AppendOnly
	// If set, the document contains signatures that may be invalidated
	// if the PDF file is saved (written) in a way that alters its previous
	// contents, as opposed to an incremental update. Merely updating
	// the PDF file by appending new information to the end of the
	// previous version is safe (see H.7, "Updating example").
	// Interactive PDF processors may use this flag to inform a user
	// requesting a full save that signatures will be invalidated and
	// require explicit confirmation before continuing with the
	// operation.
	//
	// Set SigFlags and Permissions based on Signature Type
	switch context.SignData.Signature.CertType {
	case CertificationSignature, ApprovalSignature, TimeStampSignature:
		catalog_buffer.WriteString("    /SigFlags 3\n")
	case UsageRightsSignature:
		catalog_buffer.WriteString("    /SigFlags 1\n")
	}

	// Finalize the AcroForm and Catalog object
	catalog_buffer.WriteString("  >>\n") // Close AcroForm
	catalog_buffer.WriteString(">>\n")   // Close Catalog

	return catalog_buffer.Bytes(), nil
}

// resolveExistingSignatureField returns the terminal AcroForm field of type /Sig
// whose /T text equals name and which has no /V yet. A field that already carries
// a /V (a completed prior signature) is left untouched.
func (context *SignContext) resolveExistingSignatureField(name string) (pdf.Value, bool) {
	if name == "" {
		return pdf.Value{}, false
	}
	acroForm := context.PDFReader.Trailer().Key("Root").Key("AcroForm")
	if acroForm.IsNull() {
		return pdf.Value{}, false
	}
	fields := acroForm.Key("Fields")
	for i := 0; i < fields.Len(); i++ {
		field := fields.Index(i)
		if field.Key("FT").Name() != "Sig" {
			continue
		}
		if field.Key("T").Text() != name {
			continue
		}
		if !field.Key("V").IsNull() {
			continue
		}
		return field, true
	}
	return pdf.Value{}, false
}

// buildFilledSignatureField re-serializes an existing empty signature field,
// preserving all of its entries (title, rectangle, appearance, page link) and
// adding a /V reference to the freshly written signature value dictionary. The
// result is written back at the field's own object number as an incremental
// update, so validators resolve the field's /V to the signature.
func (context *SignContext) buildFilledSignatureField(field pdf.Value) []byte {
	var buffer bytes.Buffer
	fieldID := field.GetPtr().GetID()

	buffer.WriteString("<<\n")
	for _, key := range field.Keys() {
		if key == "V" {
			continue
		}
		_, _ = fmt.Fprintf(&buffer, "  /%s ", key)
		context.serializeFieldEntry(&buffer, fieldID, field.Key(key))
		buffer.WriteString("\n")
	}
	_, _ = fmt.Fprintf(&buffer, "  /V %d 0 R\n", context.SignData.objectId)
	buffer.WriteString(">>\n")

	return buffer.Bytes()
}

// serializeFieldEntry writes value using pdfString for text strings so field
// titles keep their escaping and UTF-16 encoding. Indirect references (relative
// to the owning field object fieldID) are emitted as object references.
func (context *SignContext) serializeFieldEntry(w io.Writer, fieldID uint32, value pdf.Value) {
	if ptr := value.GetPtr(); ptr.GetID() != 0 && ptr.GetID() != fieldID {
		_, _ = fmt.Fprintf(w, "%d %d R", ptr.GetID(), ptr.GetGen())
		return
	}

	switch value.Kind() {
	case pdf.String:
		_, _ = fmt.Fprint(w, pdfString(value.Text()))
	case pdf.Null:
		_, _ = fmt.Fprint(w, "null")
	case pdf.Bool:
		if value.Bool() {
			_, _ = fmt.Fprint(w, "true")
		} else {
			_, _ = fmt.Fprint(w, "false")
		}
	case pdf.Integer:
		_, _ = fmt.Fprintf(w, "%d", value.Int64())
	case pdf.Real:
		_, _ = fmt.Fprintf(w, "%f", value.Float64())
	case pdf.Name:
		_, _ = fmt.Fprintf(w, "/%s", value.Name())
	case pdf.Dict:
		_, _ = fmt.Fprint(w, "<<")
		for idx, key := range value.Keys() {
			if idx > 0 {
				_, _ = fmt.Fprint(w, " ")
			}
			_, _ = fmt.Fprintf(w, "/%s ", key)
			context.serializeFieldEntry(w, fieldID, value.Key(key))
		}
		_, _ = fmt.Fprint(w, ">>")
	case pdf.Array:
		_, _ = fmt.Fprint(w, "[")
		for idx := 0; idx < value.Len(); idx++ {
			if idx > 0 {
				_, _ = fmt.Fprint(w, " ")
			}
			context.serializeFieldEntry(w, fieldID, value.Index(idx))
		}
		_, _ = fmt.Fprint(w, "]")
	case pdf.Stream:
		panic("stream cannot be a direct field entry")
	}
}

// serializeCatalogEntry takes a pdf.Value and serializes it to the given writer.
func (context *SignContext) serializeCatalogEntry(w io.Writer, rootObjId uint32, value pdf.Value) {
	if ptr := value.GetPtr(); ptr.GetID() != rootObjId {
		// Indirect object
		_, _ = fmt.Fprintf(w, "%d %d R", ptr.GetID(), ptr.GetGen())
		return
	}

	// Direct object
	switch value.Kind() {
	case pdf.String:
		_, _ = fmt.Fprintf(w, "(%s)", value.RawString())
	case pdf.Null:
		_, _ = fmt.Fprint(w, "null")
	case pdf.Bool:
		if value.Bool() {
			_, _ = fmt.Fprint(w, "true")
		} else {
			_, _ = fmt.Fprint(w, "false")
		}
	case pdf.Integer:
		_, _ = fmt.Fprintf(w, "%d", value.Int64())
	case pdf.Real:
		_, _ = fmt.Fprintf(w, "%f", value.Float64())
	case pdf.Name:
		_, _ = fmt.Fprintf(w, "/%s", value.Name())
	case pdf.Dict:
		_, _ = fmt.Fprint(w, "<<")
		for idx, key := range value.Keys() {
			if idx > 0 {
				_, _ = fmt.Fprint(w, " ") // Space between items
			}
			_, _ = fmt.Fprintf(w, "/%s ", key)
			context.serializeCatalogEntry(w, rootObjId, value.Key(key))
		}
		_, _ = fmt.Fprint(w, ">>")
	case pdf.Array:
		_, _ = fmt.Fprint(w, "[")
		for idx := range value.Len() {
			if idx > 0 {
				_, _ = fmt.Fprint(w, " ") // Space between items
			}
			context.serializeCatalogEntry(w, rootObjId, value.Index(idx))
		}
		_, _ = fmt.Fprint(w, "]")
	case pdf.Stream:
		panic("stream cannot be a direct object")
	}
}

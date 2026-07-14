package sign

import (
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pkcs7"

	"example.com/m/V2/internal/pdfsign/revocation"
	"github.com/mattetti/filebuffer"
)

func SignFile(input string, output string, sign_data SignData) error {
	input_file, err := os.Open(input)
	if err != nil {
		return err
	}
	defer func() {
		_ = input_file.Close()
	}()

	output_file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() {
		_ = output_file.Close()
	}()

	finfo, err := input_file.Stat()
	if err != nil {
		return err
	}
	size := finfo.Size()

	rdr, err := pdf.NewReader(input_file, size)
	if err != nil {
		return err
	}

	return Sign(input_file, output_file, rdr, size, sign_data)
}

func Sign(input io.ReadSeeker, output io.Writer, rdr *pdf.Reader, size int64, sign_data SignData) error {
	sign_data.objectId = uint32(rdr.XrefInformation.ItemCount) + 2

	context := SignContext{
		PDFReader:              rdr,
		InputFile:              input,
		OutputFile:             output,
		SignData:               sign_data,
		SignatureMaxLengthBase: uint32(hex.EncodedLen(512)),
	}

	// Fetch existing signatures
	existingSignatures, err := context.fetchExistingSignatures()
	if err != nil {
		return err
	}
	context.existingSignatures = existingSignatures

	err = context.SignPDF()
	if err != nil {
		return err
	}

	// SignPDF may rebuild context.OutputBuffer several times (replaceSignature
	// grows the placeholder and re-runs it). The finished buffer is written to
	// the output exactly once here so a retried signing pass cannot append a
	// second copy of the document.
	if _, err := context.OutputBuffer.Seek(0, 0); err != nil {
		return err
	}
	if _, err := context.OutputFile.Write(context.OutputBuffer.Buff.Bytes()); err != nil {
		return err
	}

	return nil
}

func (context *SignContext) SignPDF() error {
	// replaceSignature grows SignatureMaxLengthBase and re-invokes SignPDF when
	// the produced signature does not fit the reserved placeholder. Each pass
	// rebuilds the whole incremental update from the original input, so the
	// per-pass accumulators must start empty; carrying them over from a prior
	// pass would emit an xref whose offsets and object numbers reference the
	// discarded attempt's bytes. SignatureMaxLengthBase and existingSignatures
	// deliberately persist across the retry.
	context.newXrefEntries = nil
	context.updatedXrefEntries = nil
	context.filledFieldObjectID = 0
	context.lastXrefID = 0
	context.VisualSignData = VisualSignData{}
	context.CatalogData = CatalogData{}
	context.ByteRangeValues = nil
	context.NewXrefStart = 0
	context.SignData.RevocationData = revocation.InfoArchival{}

	// set defaults
	if context.SignData.Signature.CertType == 0 {
		context.SignData.Signature.CertType = 1
	}
	if context.SignData.Signature.DocMDPPerm == 0 {
		context.SignData.Signature.DocMDPPerm = 1
	}
	if !context.SignData.DigestAlgorithm.Available() {
		context.SignData.DigestAlgorithm = crypto.SHA256
	}
	if context.SignData.Appearance.Page == 0 {
		context.SignData.Appearance.Page = 1
	}

	context.OutputBuffer = filebuffer.New([]byte{})

	// Copy old file into new buffer.
	_, err := context.InputFile.Seek(0, 0)
	if err != nil {
		return err
	}
	if _, err := io.Copy(context.OutputBuffer, context.InputFile); err != nil {
		return err
	}

	// File always needs an empty line after %%EOF.
	if _, err := context.OutputBuffer.Write([]byte("\n")); err != nil {
		return err
	}

	// Base size for signature.
	context.SignatureMaxLength = context.SignatureMaxLengthBase

	// If not a timestamp signature
	if context.SignData.Signature.CertType != TimeStampSignature {
		if context.SignData.Certificate == nil {
			return fmt.Errorf("certificate is required")
		}

		context.SignatureMaxLength += signatureAlgorithmMaxLengthHint(context.SignData.Certificate.SignatureAlgorithm.String())

		// Add size of digest algorithm twice (for file digist and signing certificate attribute)
		context.SignatureMaxLength += uint32(hex.EncodedLen(context.SignData.DigestAlgorithm.Size() * 2))

		// Add size for my certificate.
		degenerated, err := pkcs7.DegenerateCertificate(context.SignData.Certificate.Raw)
		if err != nil {
			return fmt.Errorf("failed to degenerate certificate: %w", err)
		}

		context.SignatureMaxLength += uint32(hex.EncodedLen(len(degenerated)))

		// Add size of the raw issuer which is added by AddSignerChain
		context.SignatureMaxLength += uint32(hex.EncodedLen(len(context.SignData.Certificate.RawIssuer)))

		// Add size for certificate chain.
		var certificate_chain []*x509.Certificate
		if len(context.SignData.CertificateChains) > 0 && len(context.SignData.CertificateChains[0]) > 1 {
			certificate_chain = context.SignData.CertificateChains[0][1:]
		}

		if len(certificate_chain) > 0 {
			for _, cert := range certificate_chain {
				degenerated, err := pkcs7.DegenerateCertificate(cert.Raw)
				if err != nil {
					return fmt.Errorf("failed to degenerate certificate in chain: %w", err)
				}

				context.SignatureMaxLength += uint32(hex.EncodedLen(len(degenerated)))
			}
		}

		// Fetch revocation data before adding signature placeholder.
		// Revocation data can be quite large and we need to create enough space in the placeholder.
		if err := context.fetchRevocationData(); err != nil {
			return fmt.Errorf("failed to fetch revocation data: %w", err)
		}
	}

	// Add estimated size for TSA.
	// We can't kow actual size of TSA until after signing.
	//
	// Different TSA servers provide different response sizes, we
	// might need to make this configurable or detect and store.
	if context.SignData.TSA.URL != "" {
		context.SignatureMaxLength += uint32(hex.EncodedLen(9000))
	}

	// Create the signature object
	var signature_object []byte

	switch context.SignData.Signature.CertType {
	case TimeStampSignature:
		signature_object = context.createTimestampPlaceholder()
	default:
		signature_object = context.createSignaturePlaceholder()
	}

	// Write the new signature object
	context.SignData.objectId, err = context.addObject(signature_object)
	if err != nil {
		return fmt.Errorf("failed to add signature object: %w", err)
	}

	// When the input PDF already carries an empty signature field with the named
	// title, fill that field's /V via an incremental update so the signature is
	// linked to the pre-rendered AcroForm field instead of being appended as a
	// second, unlinked field. When no such field exists a new one is created.
	field, hasField := context.resolveExistingSignatureField(context.SignData.ExistingSignatureFieldName)
	if hasField {
		fieldObjectID := field.GetPtr().GetID()
		filled := context.buildFilledSignatureField(field)
		if err := context.updateObject(fieldObjectID, filled); err != nil {
			return fmt.Errorf("failed to fill existing signature field: %w", err)
		}
		context.filledFieldObjectID = fieldObjectID
		context.VisualSignData.objectId = fieldObjectID
	} else {
		// Create visual signature (visible or invisible based on CertType)
		visible := false
		rectangle := [4]float64{0, 0, 0, 0}
		if context.SignData.Signature.CertType != ApprovalSignature && context.SignData.Appearance.Visible {
			return fmt.Errorf("visible signatures are only allowed for approval signatures")
		} else if context.SignData.Signature.CertType == ApprovalSignature && context.SignData.Appearance.Visible {
			visible = true
			rectangle = [4]float64{
				context.SignData.Appearance.LowerLeftX,
				context.SignData.Appearance.LowerLeftY,
				context.SignData.Appearance.UpperRightX,
				context.SignData.Appearance.UpperRightY,
			}
		}

		// Example usage: passing page number and default rect values
		visual_signature, err := context.createVisualSignature(visible, context.SignData.Appearance.Page, rectangle)
		if err != nil {
			return fmt.Errorf("failed to create visual signature: %w", err)
		}

		// Write the new visual signature object.
		context.VisualSignData.objectId, err = context.addObject(visual_signature)
		if err != nil {
			return fmt.Errorf("failed to add visual signature object: %w", err)
		}
	}

	if context.SignData.Appearance.Visible && context.filledFieldObjectID == 0 {
		inc_page_update, err := context.createIncPageUpdate(context.SignData.Appearance.Page, context.VisualSignData.objectId)
		if err != nil {
			return fmt.Errorf("failed to create incremental page update: %w", err)
		}
		err = context.updateObject(context.VisualSignData.pageObjectId, inc_page_update)
		if err != nil {
			return fmt.Errorf("failed to add incremental page update object: %w", err)
		}
	}

	// Create a new catalog object
	catalog, err := context.createCatalog()
	if err != nil {
		return fmt.Errorf("failed to create catalog: %w", err)
	}

	// Write the new catalog object
	context.CatalogData.ObjectId, err = context.addObject(catalog)
	if err != nil {
		return fmt.Errorf("failed to add catalog object: %w", err)
	}

	// Write xref table
	if err := context.writeXref(); err != nil {
		return fmt.Errorf("failed to write xref: %w", err)
	}

	// Write trailer
	if err := context.writeTrailer(); err != nil {
		return fmt.Errorf("failed to write trailer: %w", err)
	}

	// Update byte range
	if err := context.updateByteRange(); err != nil {
		return fmt.Errorf("failed to update byte range: %w", err)
	}

	// Replace signature
	if err := context.replaceSignature(); err != nil {
		return fmt.Errorf("failed to replace signature: %w", err)
	}

	return nil
}

// signatureAlgorithmMaxLengthHint returns the extra byte-length headroom to
// reserve in the signature placeholder for a certificate's signature
// algorithm family (hex-encoded, hence [hex.EncodedLen]). sigAlg is the
// string form of an [x509.SignatureAlgorithm] (e.g. "SHA256-RSA"). Unknown
// algorithms add no extra headroom.
//
// Grouped with comma-joined case lists rather than one case per algorithm
// name: Go switch statements do not fall through between cases, so listing
// "SHA1-RSA" and "ECDSA-SHA1" as separate empty cases above a shared body
// (the original, buggy form of this function) silently reserves zero extra
// bytes for every algorithm except the last name in each group.
func signatureAlgorithmMaxLengthHint(sigAlg string) uint32 {
	switch sigAlg {
	case "SHA1-RSA", "ECDSA-SHA1", "DSA-SHA1":
		return uint32(hex.EncodedLen(128))
	case "SHA256-RSA", "ECDSA-SHA256", "DSA-SHA256":
		return uint32(hex.EncodedLen(256))
	case "SHA384-RSA", "ECDSA-SHA384":
		return uint32(hex.EncodedLen(384))
	case "SHA512-RSA", "ECDSA-SHA512":
		return uint32(hex.EncodedLen(512))
	default:
		return 0
	}
}

package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

type PDFStateData struct {
	IPFSCID         string
	RendererVersion string
	C2PAState       string
}

type pdfStateUpdater func(ctx context.Context, tx *sqlx.Tx, did string, state PDFStateData) error

// pdfSignatureNotAvailable is the honest PDF-signature check status reported by
// the verify endpoint while Workstream B/PAdES is not implemented yet
// (DCS-OR-C2PA-006 AC6). The verifier must never falsely report a passed PDF
// signature check for a capability that does not exist.
const pdfSignatureNotAvailable = "not_available"

func appendAndCache(
	ctx context.Context,
	tx *sqlx.Tx,
	did, state string,
	jsonldBytes, pdfBytes []byte,
	ipfsClient *ipfs.APIClient,
	pdfCore *pdfcore.Client,
	vcIssuer provenance.VCIssuer,
	issuerDID string,
	updateState pdfStateUpdater,
) ([]byte, error) {
	c2paState, err := provenance.MapCWEStateToC2PA(state)
	if err != nil {
		return pdfBytes, fmt.Errorf("map lifecycle state %q: %w", state, err)
	}

	log.Printf("pdfgeneration: appendAndCache %s state=%s c2paState=%s pdfLen=%d",
		did, state, c2paState, len(pdfBytes))

	reason := stateToReason(c2paState)

	h := sha256.Sum256(pdfBytes)
	assetHash := hex.EncodeToString(h[:])

	_, vcBytes, err := vcIssuer.IssueContractLifecycleVC(
		ctx, did, assetHash, c2paState, reason, issuerDID, time.Now().UTC(),
	)
	if err != nil {
		return pdfBytes, fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	updatedPDF, rendererVersion, err := pdfCore.Update(ctx, pdfBytes, jsonldBytes, vcBytes, provenance.RemoteManifestURL(did))
	if err != nil {
		return pdfBytes, fmt.Errorf("pdf-core update for %s: %w", did, err)
	}

	ipfsResult, err := ipfsClient.CreateFile(ctx, updatedPDF)
	if err != nil {
		return updatedPDF, fmt.Errorf("store PDF in IPFS for %s: %w", did, err)
	}
	pdfCID := ipfsResult.Identifier.Value

	if err := updateState(ctx, tx, did, PDFStateData{
		IPFSCID:         pdfCID,
		RendererVersion: rendererVersion,
		C2PAState:       c2paState,
	}); err != nil {
		return nil, fmt.Errorf("persist PDF state for %s: %w", did, err)
	}

	log.Printf("pdfgeneration: appendAndCache %s done → CID=%s pdfLen=%d", did, pdfCID, len(updatedPDF))
	return updatedPDF, nil
}

func runVerify(ctx context.Context, pdfBytes []byte, pdfCore *pdfcore.Client, lifecycleStatus string) (*pdfgen.PDFVerifyResult, error) {
	result, verifyErr := pdfCore.Verify(ctx, pdfBytes)
	match := verifyErr == nil
	c2paManifestFound := verifyErr == nil || (verifyErr != nil && strings.Contains(verifyErr.Error(), "status 409"))
	c2paSignatureValid := verifyErr == nil

	statusListURI := ""
	statusListStatus := ""
	if result.VCProofValid && len(result.VCBytes) > 0 {
		statusListURI = provenance.ExtractStatusListURI(result.VCBytes)
		if cred, idx, ok := provenance.ExtractCredentialStatusFields(result.VCBytes); ok {
			httpClient := &http.Client{Timeout: 10 * time.Second}
			if status, err := provenance.QueryStatusListStatus(ctx, httpClient, cred, idx); err == nil {
				statusListStatus = status
			}
		}
	}

	return &pdfgen.PDFVerifyResult{
		Match:              match,
		C2paManifestFound:  c2paManifestFound,
		C2paSignatureValid: c2paSignatureValid,
		VcProofValid:       result.VCProofValid,
		StatusListURI:      ptrToString(statusListURI),
		StatusListStatus:   ptrToString(statusListStatus),
		LifecycleStatus:    ptrToString(lifecycleStatus),
		// DCS-OR-C2PA-006 AC6: the PDF-signature check is an independently named
		// check, distinct from the C2PA COSE signature check. Workstream B/PAdES
		// has not landed yet, so we honestly report "not_available" rather than
		// faking a passed PDF-signature verification.
		PdfSignatureStatus: pdfSignatureNotAvailable,
	}, nil
}

func stateToReason(state string) string {
	switch state {
	case "draft":
		return "Contract created as draft"
	case "active":
		return "Contract activated for execution"
	case "amended":
		return "Contract amended with new terms"
	case "suspended":
		return "Contract suspended pending review"
	case "terminated":
		return "Contract terminated by parties"
	case "expired":
		return "Contract reached expiration date"
	case "replaced":
		return "Contract replaced with newer version"
	default:
		return "Contract state changed to: " + state
	}
}

func ptrToString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

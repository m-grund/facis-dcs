package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/signingmanagement/command"
	db "digital-contracting-service/internal/signingmanagement/db"
)

// signingRequestTTL is how long a published OID4VP signing request stays valid
// for a wallet to fetch, sign, and post the signed document back.
const signingRequestTTL = 15 * time.Minute

// signatureQualifierFor maps a DCS credential type to the CSC/rQES
// signatureQualifier the wallet honours (the value the EUDI walletdriven-signer
// advertises in the request object). QES is descoped (SRS §199); an unknown type
// defaults to the AES qualifier.
func signatureQualifierFor(credentialType string) string {
	if strings.EqualFold(credentialType, "QES") {
		return "eu_eidas_qes"
	}
	return "eu_eidas_aes"
}

// PublishSignatureRequest runs Applier.Prepare to produce the to-be-signed PDF
// for a verified ceremony, stores it (so the wallet signs exactly the committed
// bytes), and returns the OID4VP Document-Retrieval request as QR/deep-link data
// (ADR-12). The request object itself is served, by reference, from
// GET .../object; the wallet fetches it, fetches the document it references,
// signs, and posts the signed document back to the callback.
func (s *signatureManagementsrvc) PublishSignatureRequest(ctx context.Context, req *signaturemanagement.SMSignatureRequestPublishRequest) (res *signaturemanagement.SMSignatureRequestPublishResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if s.RequestSigner == nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("OID4VP request signer is not configured"))
	}
	if strings.TrimSpace(s.PublicAPIBase) == "" {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("public API base URL is not configured"))
	}

	ceremony, err := s.getCeremony(ctx, req.CeremonyID)
	if err != nil {
		return nil, err
	}
	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", req.CeremonyID))
	}
	if ceremony.Status != db.CeremonyVerified || ceremony.SignerDID == nil || strings.TrimSpace(*ceremony.SignerDID) == "" {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony %s has no verified PID presentation to publish a signing request for", req.CeremonyID))
	}

	credentialType := "AES"
	if req.CredentialType != nil && *req.CredentialType != "" {
		credentialType = *req.CredentialType
	}

	appliedBy := middleware.GetParticipantID(ctx)
	holderDID := middleware.GetHolderDID(ctx)
	roles := middleware.GetUserRoles(ctx)
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("encode signer roles: %w", err))
	}

	// Prepare seals the agreement, embeds the signing-summary evidence, and
	// places the AcroForm field, yielding the to-be-signed PDF (it holds no
	// signing key). This is the exact same preparation /signature/prepare runs.
	applier := s.newApplier()
	document, err := applier.Prepare(ctx, command.ApplyCmd{
		DID:            ceremony.ContractDID,
		SignerDID:      *ceremony.SignerDID,
		FieldName:      ceremony.FieldName,
		CredentialType: credentialType,
		AppliedBy:      appliedBy,
		HolderDID:      holderDID,
		UserRoles:      roles,
	})
	if err != nil {
		return nil, mapSignatureCommandError(err)
	}

	sum := sha256.Sum256(document)
	digestHex := hex.EncodeToString(sum[:])
	nonce := uuid.NewString()
	expiresAt := time.Now().UTC().Add(signingRequestTTL)

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.CeremonyRepo.StorePreparedRequest(ctx, tx, db.PreparedRequest{
		CeremonyID:        ceremony.ID,
		PreparedPDF:       document,
		PreparedPDFSHA256: digestHex,
		RequestNonce:      nonce,
		RequestExpiresAt:  expiresAt,
		CredentialType:    credentialType,
		PublishedBy:       appliedBy,
		HolderDID:         holderDID,
		Roles:             rolesJSON,
	}); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	requestURI := s.signatureRequestURL(ceremony.ID, "object")
	return &signaturemanagement.SMSignatureRequestPublishResponse{
		CeremonyID: ceremony.ID,
		ClientID:   s.OID4VPClientID,
		RequestURI: requestURI,
		WalletURI:  buildOpenID4VPPresentationURI(s.OID4VPClientID, requestURI),
		Nonce:      &nonce,
		ExpiresAt:  expiresAt.Format(time.RFC3339),
	}, nil
}

// SignatureRequestObject serves the signed OID4VP Document-Retrieval request
// object (JAR) the wallet fetches by reference — built from the ceremony's stored
// digest, nonce, and expiry, exactly like authSvc.PresentationRequest serves the
// login/PID JAR.
func (s *signatureManagementsrvc) SignatureRequestObject(ctx context.Context, p *signaturemanagement.SignatureRequestObjectPayload) (io.ReadCloser, error) {
	ceremony, err := s.loadPublishedCeremony(ctx, p.CeremonyID)
	if err != nil {
		return nil, err
	}

	digestBytes, decErr := hex.DecodeString(*ceremony.PreparedPDFSHA256)
	if decErr != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("decode prepared document digest: %w", decErr))
	}

	credentialType := "AES"
	if ceremony.CredentialType != nil && *ceremony.CredentialType != "" {
		credentialType = *ceremony.CredentialType
	}

	jwt, err := oid4vprequest.BuildDocumentRetrievalJWT(s.RequestSigner, oid4vprequest.DocRetrievalParams{
		ClientID:           s.OID4VPClientID,
		ResponseURI:        s.signatureRequestURL(ceremony.ID, "callback"),
		Nonce:              *ceremony.RequestNonce,
		ExpiresAt:          *ceremony.RequestExpiresAt,
		SignatureQualifier: signatureQualifierFor(credentialType),
		DocumentDigests: []oid4vprequest.DocumentDigest{
			{Label: ceremony.FieldName, Hash: base64.StdEncoding.EncodeToString(digestBytes)},
		},
		DocumentLocations: []oid4vprequest.DocumentLocation{
			{URI: s.signatureRequestURL(ceremony.ID, "document"), Method: oid4vprequest.DocumentLocationMethod{Type: "public"}},
		},
	})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("build signing request object: %w", err))
	}
	return io.NopCloser(bytes.NewReader([]byte(jwt))), nil
}

// SignatureRequestDocument serves the stored to-be-signed PDF the wallet fetches
// from the request object's document_locations.
func (s *signatureManagementsrvc) SignatureRequestDocument(ctx context.Context, p *signaturemanagement.SignatureRequestDocumentPayload) (io.ReadCloser, error) {
	ceremony, err := s.loadPublishedCeremony(ctx, p.CeremonyID)
	if err != nil {
		return nil, err
	}
	if len(ceremony.PreparedPDF) == 0 {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s has no prepared document", p.CeremonyID))
	}
	return io.NopCloser(bytes.NewReader(ceremony.PreparedPDF)), nil
}

// SignatureRequestCallback accepts the wallet's signed document at the request
// object's response_uri and finalizes the contract, reusing the exact validate +
// finalize path /signature/submit uses (Applier.SubmitSignature): the DSS
// sole-control gate then Applier.finalize. The publishing signer's participant
// context, captured at publish, is replayed so the JWT-less callback attributes
// the signature correctly. The published request is single-use.
func (s *signatureManagementsrvc) SignatureRequestCallback(ctx context.Context, p *signaturemanagement.SignatureRequestCallbackPayload, body io.ReadCloser) (res *signaturemanagement.SMSignatureRequestCallbackResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	form, err := parseDirectPostForm(body)
	if err != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("parse direct_post body: %w", err))
	}
	if walletErr := strings.TrimSpace(form.Get("error")); walletErr != "" {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("wallet reported a signing error: %s", walletErr))
	}
	signedDocs := formList(form, "documentWithSignature")
	if len(signedDocs) == 0 {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("no documentWithSignature was posted"))
	}
	signedPDF, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(signedDocs[0]))
	if decErr != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("decode signed document: %w", decErr))
	}
	if state := strings.TrimSpace(form.Get("state")); state != "" && state != p.CeremonyID {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("callback state %q does not match ceremony %s", state, p.CeremonyID))
	}

	ceremony, err := s.loadPublishedCeremony(ctx, p.CeremonyID)
	if err != nil {
		return nil, err
	}
	if ceremony.ConsumedAt != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony %s signing request has already been consumed", p.CeremonyID))
	}

	credentialType := "AES"
	if ceremony.CredentialType != nil && *ceremony.CredentialType != "" {
		credentialType = *ceremony.CredentialType
	}
	// A detached JAdES over the machine-readable JSON-LD rides in the EUDI
	// signatureObject[] list (the PAdES itself is enveloped in the PDF).
	jades := ""
	if objects := formList(form, "signatureObject"); len(objects) > 0 {
		jades = strings.TrimSpace(objects[0])
	}
	appliedBy := ""
	if ceremony.PublishedBy != nil {
		appliedBy = *ceremony.PublishedBy
	}
	holderDID := ""
	if ceremony.PublishedHolderDID != nil {
		holderDID = *ceremony.PublishedHolderDID
	}
	var roles userrole.UserRoles
	if len(ceremony.PublishedRoles) > 0 {
		if err := json.Unmarshal(ceremony.PublishedRoles, &roles); err != nil {
			return nil, signaturemanagement.MakeInternalError(fmt.Errorf("decode publisher roles: %w", err))
		}
	}

	// The signature field is the participating party (org DID); the natural person
	// who signs is established separately, by the ceremony's verified PID. It is
	// NOT established by the signing certificate: AssertValidAES checks that the
	// signature is a valid AES and nothing more — no PID-to-certificate identifier
	// binding is standardised (see apply.go's SubmitSignature).
	applier := s.newApplier()
	if err := applier.SubmitSignature(ctx, command.SubmitSignatureCmd{
		ApplyCmd: command.ApplyCmd{
			DID:            ceremony.ContractDID,
			SignerDID:      *ceremony.SignerDID,
			FieldName:      ceremony.FieldName,
			CredentialType: credentialType,
			AppliedBy:      appliedBy,
			HolderDID:      holderDID,
			UserRoles:      roles,
		},
		SignedPDF:      signedPDF,
		JAdESSignature: jades,
	}); err != nil {
		return nil, mapSignatureCommandError(err)
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.CeremonyRepo.MarkCeremonyConsumed(ctx, tx, ceremony.ID); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	processData, err := s.CRepo.ReadProcessDataByDID(ctx, tx, ceremony.ContractDID)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	did := ceremony.ContractDID
	return &signaturemanagement.SMSignatureRequestCallbackResponse{
		CeremonyID: ceremony.ID,
		Did:        &did,
		Status:     processData.State,
	}, nil
}

// getCeremony reads a ceremony by id in a short read transaction.
func (s *signatureManagementsrvc) getCeremony(ctx context.Context, id string) (*db.SignatureCeremony, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	ceremony, err := s.CeremonyRepo.GetCeremonyByID(ctx, tx, id)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	return ceremony, nil
}

// loadPublishedCeremony resolves a ceremony that has a live published signing
// request (a prepared document, a fresh nonce, and an unexpired request), the
// precondition the object/document/callback endpoints share.
func (s *signatureManagementsrvc) loadPublishedCeremony(ctx context.Context, id string) (*db.SignatureCeremony, error) {
	ceremony, err := s.getCeremony(ctx, id)
	if err != nil {
		return nil, err
	}
	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", id))
	}
	if ceremony.PreparedPDFSHA256 == nil || ceremony.RequestNonce == nil || ceremony.RequestExpiresAt == nil || ceremony.SignerDID == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s has no published signing request", id))
	}
	if time.Now().UTC().After(*ceremony.RequestExpiresAt) {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony %s signing request has expired", id))
	}
	return ceremony, nil
}

// signatureRequestURL builds an absolute per-ceremony signing-request endpoint
// URL on the public API base.
func (s *signatureManagementsrvc) signatureRequestURL(ceremonyID, leaf string) string {
	return strings.TrimRight(s.PublicAPIBase, "/") + "/signature/request/" + url.PathEscape(ceremonyID) + "/" + leaf
}

// directPostMaxBytes bounds the wallet's direct_post body; a signed contract PDF
// with embedded evidence is a few MB.
const directPostMaxBytes = 64 << 20

// parseDirectPostForm reads and url-decodes the wallet's application/
// x-www-form-urlencoded direct_post body (the EUDI walletdriven-signer response).
func parseDirectPostForm(body io.ReadCloser) (url.Values, error) {
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, directPostMaxBytes))
	if err != nil {
		return nil, err
	}
	return url.ParseQuery(string(raw))
}

// formList extracts a repeated form field the way the EUDI walletdriven-signer
// relying party does (retrieve_list_values_from_form_urlencoded): indexed keys
// (name[0], name[]) first, then repeated bare keys.
func formList(form url.Values, name string) []string {
	var indexed []string
	for key, values := range form {
		if strings.HasPrefix(key, name+"[") {
			indexed = append(indexed, values...)
		}
	}
	if len(indexed) > 0 {
		return indexed
	}
	return form[name]
}

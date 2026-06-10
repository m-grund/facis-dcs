package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strings"

	compiler "example.com/m/V2/compiler"
	dcspdfcore "example.com/m/V2/gen/dcspdfcore"
)

// dcspdfcoreService implements dcspdfcore.Service.
type dcspdfcoreService struct{}

// checkMediaType returns an unsupported_media_type error if the Content-Type
// header is not in the allowed set. Allowed values are matched on the bare
// media type (parameters such as charset are ignored).
func checkMediaType(contentType string, allowed ...string) error {
	if contentType == "" {
		return dcspdfcore.MakeUnsupportedMediaType(errors.New("missing content type"))
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return dcspdfcore.MakeUnsupportedMediaType(fmt.Errorf("invalid content type: %w", err))
	}
	for _, a := range allowed {
		if mediaType == a {
			return nil
		}
	}
	return dcspdfcore.MakeUnsupportedMediaType(
		fmt.Errorf("unsupported content type %q", mediaType),
	)
}

// limitRead reads up to limit bytes and errors if the body is empty.
func limitRead(r io.ReadCloser, limit int64) ([]byte, error) {
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("request body is empty")
	}
	return b, nil
}

// Download compiles a JSON-LD payload into deterministic PDF/A-3a bytes.
func (s *dcspdfcoreService) Download(_ context.Context, p *dcspdfcore.DownloadPayload, body io.ReadCloser) ([]byte, error) {
	if err := checkMediaType(p.ContentType, "application/ld+json", "application/json"); err != nil {
		return nil, err
	}
	raw, err := limitRead(body, 8<<20)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}
	canonical, err := compiler.CanonicalizePayload(raw)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}
	pdf, err := compiler.CompilePDF(canonical)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}
	return pdf, nil
}

// Verify checks that a PDF was compiled deterministically from its embedded
// payload and appends a C2PA verification witness.
//
// For a plain compiled PDF the guarantee is:
//
//	CompilePDF(embeddedPayload) == submittedPDF
//
// For an incrementally amended PDF the guarantee is extended to cover the full
// provenance chain:
//
//	CompilePDF(oldPayload)          == originalPrefix
//	UpdatePDF(originalPrefix, newPayload) == submittedPDF
//
// Both conditions together prove that the human-readable content is fully
// determined by the machine-readable JSON-LD at every revision.
func (s *dcspdfcoreService) Verify(_ context.Context, p *dcspdfcore.VerifyPayload, body io.ReadCloser) ([]byte, error) {
	if err := checkMediaType(p.ContentType, "application/pdf"); err != nil {
		return nil, err
	}
	raw, err := limitRead(body, 32<<20)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}

	var payload []byte

	if _, ok := compiler.SplitAtIncrementalUpdate(raw); ok {
		// Amended PDF: verify the full provenance chain.
		if err := compiler.VerifyIncrementalUpdate(raw); err != nil {
			return nil, dcspdfcore.MakeConflict(err)
		}
		payload, err = compiler.ExtractLatestEmbeddedJSONLD(raw)
		if err != nil {
			return nil, dcspdfcore.MakeBadRequest(err)
		}
	} else {
		// Plain compiled PDF: verify the single-step determinism guarantee.
		payload, err = compiler.ExtractEmbeddedJSONLD(raw)
		if err != nil {
			return nil, dcspdfcore.MakeBadRequest(err)
		}
		recompiled, err := compiler.CompilePDF(payload)
		if err != nil {
			return nil, dcspdfcore.MakeUnprocessableEntity(err)
		}
		if !bytes.Equal(raw, recompiled) {
			return nil, dcspdfcore.MakeConflict(errors.New("embedded payload does not reproduce the submitted PDF"))
		}
	}

	verified, err := compiler.AppendVerificationWitness(raw, payload)
	if err != nil {
		return nil, err
	}
	return verified, nil
}

// Update amends an existing PDF with a new JSON-LD payload.
// The request body must be multipart/form-data with fields "pdf" and "payload".
func (s *dcspdfcoreService) Update(_ context.Context, p *dcspdfcore.UpdatePayload, body io.ReadCloser) ([]byte, error) {
	if err := checkMediaType(p.ContentType, "multipart/form-data"); err != nil {
		return nil, err
	}
	defer body.Close()

	_, params, err := mime.ParseMediaType(p.ContentType)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(fmt.Errorf("invalid multipart content type: %w", err))
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, dcspdfcore.MakeBadRequest(errors.New("multipart boundary missing from Content-Type"))
	}

	parts := make(map[string][]byte)
	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, dcspdfcore.MakeBadRequest(fmt.Errorf("read multipart: %w", err))
		}
		name := part.FormName()
		var limit int64 = 8 << 20
		if name == "pdf" {
			limit = 32 << 20
		}
		data, err := io.ReadAll(io.LimitReader(part, limit))
		part.Close()
		if err != nil {
			return nil, dcspdfcore.MakeBadRequest(fmt.Errorf("read part %q: %w", name, err))
		}
		parts[name] = data
	}

	oldPDF, ok := parts["pdf"]
	if !ok || len(oldPDF) == 0 {
		return nil, dcspdfcore.MakeBadRequest(errors.New("pdf field required"))
	}
	newPayload, ok := parts["payload"]
	if !ok || len(newPayload) == 0 {
		return nil, dcspdfcore.MakeBadRequest(errors.New("payload field required"))
	}
	canonical, err := compiler.CanonicalizePayload(newPayload)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}

	updated, err := compiler.UpdatePDF(oldPDF, canonical)
	if err != nil {
		if strings.Contains(err.Error(), "no changes") {
			return nil, dcspdfcore.MakeConflict(err)
		}
		return nil, dcspdfcore.MakeBadRequest(err)
	}
	return updated, nil
}

// Claim verifies that the supplied JSON-LD payload produces the same page
// content as the submitted PDF and returns the canonical compiled PDF — with
// the JSON-LD embedded and a C2PA verification witness — as evidence of the
// match.  The submitted PDF need not contain embedded metadata; only the
// visible page content is compared.
func (s *dcspdfcoreService) Claim(_ context.Context, p *dcspdfcore.ClaimPayload, body io.ReadCloser) ([]byte, error) {
	if err := checkMediaType(p.ContentType, "multipart/form-data"); err != nil {
		return nil, err
	}
	defer body.Close()

	_, params, err := mime.ParseMediaType(p.ContentType)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(fmt.Errorf("invalid multipart content type: %w", err))
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, dcspdfcore.MakeBadRequest(errors.New("multipart boundary missing from Content-Type"))
	}

	parts := make(map[string][]byte)
	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, dcspdfcore.MakeBadRequest(fmt.Errorf("read multipart: %w", err))
		}
		name := part.FormName()
		var limit int64 = 8 << 20
		if name == "pdf" {
			limit = 32 << 20
		}
		data, err := io.ReadAll(io.LimitReader(part, limit))
		part.Close()
		if err != nil {
			return nil, dcspdfcore.MakeBadRequest(fmt.Errorf("read part %q: %w", name, err))
		}
		parts[name] = data
	}

	submittedPDF, ok := parts["pdf"]
	if !ok || len(submittedPDF) == 0 {
		return nil, dcspdfcore.MakeBadRequest(errors.New("pdf field required"))
	}
	payloadBytes, ok := parts["payload"]
	if !ok || len(payloadBytes) == 0 {
		return nil, dcspdfcore.MakeBadRequest(errors.New("payload field required"))
	}
	canonical, err := compiler.CanonicalizePayload(payloadBytes)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}

	canonicalPDF, err := compiler.CompilePDF(canonical)
	if err != nil {
		return nil, dcspdfcore.MakeBadRequest(err)
	}
	if err := compiler.MatchPageContent(submittedPDF, canonicalPDF); err != nil {
		return nil, dcspdfcore.MakeConflict(err)
	}
	result, err := compiler.AppendVerificationWitness(canonicalPDF, canonical)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// substituteBaseURL replaces the hardcoded default base URL in embedded ontology
// bytes with the configured DCS_PDF_CORE_ONTOLOGY_BASE_URL, if set. This allows
// deployments behind a public hostname to serve a usable JSON-LD context.
func substituteBaseURL(raw []byte) []byte {
	base := os.Getenv("DCS_PDF_CORE_ONTOLOGY_BASE_URL")
	if base == "" {
		return raw
	}
	base = strings.TrimRight(base, "/")
	return bytes.ReplaceAll(raw, []byte("http://127.0.0.1:8080"), []byte(base))
}

// OntologyContext serves the embedded DCS-PDF-CORE JSON-LD context.
func (s *dcspdfcoreService) OntologyContext(_ context.Context) ([]byte, error) {
	return substituteBaseURL(ontologyContext), nil
}

// OntologyOwl serves the embedded DCS-PDF-CORE OWL definition.
func (s *dcspdfcoreService) OntologyOwl(_ context.Context) ([]byte, error) {
	return substituteBaseURL(ontologyOWL), nil
}

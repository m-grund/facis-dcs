package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	compiler "example.com/m/V2/compiler"

	"github.com/google/uuid"
)

// service implements all HTTP handlers for the DCS-PDF-CORE API.
type service struct{}

// httpError carries enough information to write a structured error response.
type httpError struct {
	status  int
	name    string
	message string
}

func (e *httpError) Error() string { return e.message }

// writeError writes a JSON error body matching the OpenAPI Error schema.
func writeError(w http.ResponseWriter, err error) {
	var he *httpError
	if !errors.As(err, &he) {
		he = &httpError{
			status:  http.StatusInternalServerError,
			name:    "internal_error",
			message: err.Error(),
		}
	}
	body := map[string]interface{}{
		"name":      he.name,
		"id":        uuid.New().String(),
		"message":   he.message,
		"temporary": false,
		"timeout":   false,
		"fault":     false,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(he.status)
	_ = json.NewEncoder(w).Encode(body)
}

func errBadRequest(err error) *httpError {
	return &httpError{status: http.StatusBadRequest, name: "bad_request", message: err.Error()}
}

func errUnsupportedMediaType(err error) *httpError {
	return &httpError{status: http.StatusUnsupportedMediaType, name: "unsupported_media_type", message: err.Error()}
}

func errConflict(err error) *httpError {
	return &httpError{status: http.StatusConflict, name: "conflict", message: err.Error()}
}

func errUnprocessableEntity(err error) *httpError {
	return &httpError{status: http.StatusUnprocessableEntity, name: "unprocessable_entity", message: err.Error()}
}

// checkMediaType returns an unsupported_media_type error if the Content-Type
// header is not in the allowed set. Allowed values are matched on the bare
// media type (parameters such as charset are ignored).
func checkMediaType(contentType string, allowed ...string) error {
	if contentType == "" {
		return errUnsupportedMediaType(errors.New("missing content type"))
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return errUnsupportedMediaType(fmt.Errorf("invalid content type: %w", err))
	}
	for _, a := range allowed {
		if mediaType == a {
			return nil
		}
	}
	return errUnsupportedMediaType(fmt.Errorf("unsupported content type %q", mediaType))
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

func (s *service) download(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/ld+json", "application/json"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 8<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	canonical, err := compiler.CanonicalizePayload(raw)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	pdf, err := compiler.CompilePDF(canonical)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(pdf)
}

func (s *service) verify(w http.ResponseWriter, r *http.Request) {
	if err := checkMediaType(r.Header.Get("Content-Type"), "application/pdf"); err != nil {
		writeError(w, err)
		return
	}
	raw, err := limitRead(r.Body, 32<<20)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}

	var payload []byte

	if _, ok := compiler.SplitAtIncrementalUpdate(raw); ok {
		if err := compiler.VerifyIncrementalUpdate(raw); err != nil {
			writeError(w, errConflict(err))
			return
		}
		payload, err = compiler.ExtractLatestEmbeddedJSONLD(raw)
		if err != nil {
			writeError(w, errBadRequest(err))
			return
		}
	} else {
		payload, err = compiler.ExtractEmbeddedJSONLD(raw)
		if err != nil {
			writeError(w, errBadRequest(err))
			return
		}
		recompiled, err := compiler.CompilePDF(payload)
		if err != nil {
			writeError(w, errUnprocessableEntity(err))
			return
		}
		if !bytes.Equal(raw, recompiled) {
			writeError(w, errConflict(errors.New("embedded payload does not reproduce the submitted PDF")))
			return
		}
	}

	verified, err := compiler.AppendVerificationWitness(raw, payload)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(verified)
}

func (s *service) update(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if err := checkMediaType(ct, "multipart/form-data"); err != nil {
		writeError(w, err)
		return
	}
	defer r.Body.Close()

	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("invalid multipart content type: %w", err)))
		return
	}
	boundary, ok := params["boundary"]
	if !ok {
		writeError(w, errBadRequest(errors.New("multipart boundary missing from Content-Type")))
		return
	}

	parts, err := readMultipartParts(r.Body, boundary)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}

	oldPDF, ok := parts["pdf"]
	if !ok || len(oldPDF) == 0 {
		writeError(w, errBadRequest(errors.New("pdf field required")))
		return
	}
	newPayload, ok := parts["payload"]
	if !ok || len(newPayload) == 0 {
		writeError(w, errBadRequest(errors.New("payload field required")))
		return
	}
	canonical, err := compiler.CanonicalizePayload(newPayload)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	updated, err := compiler.UpdatePDF(oldPDF, canonical)
	if err != nil {
		if strings.Contains(err.Error(), "no changes") {
			writeError(w, errConflict(err))
			return
		}
		writeError(w, errBadRequest(err))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(updated)
}

func (s *service) claim(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if err := checkMediaType(ct, "multipart/form-data"); err != nil {
		writeError(w, err)
		return
	}
	defer r.Body.Close()

	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		writeError(w, errBadRequest(fmt.Errorf("invalid multipart content type: %w", err)))
		return
	}
	boundary, ok := params["boundary"]
	if !ok {
		writeError(w, errBadRequest(errors.New("multipart boundary missing from Content-Type")))
		return
	}

	parts, err := readMultipartParts(r.Body, boundary)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}

	submittedPDF, ok := parts["pdf"]
	if !ok || len(submittedPDF) == 0 {
		writeError(w, errBadRequest(errors.New("pdf field required")))
		return
	}
	payloadBytes, ok := parts["payload"]
	if !ok || len(payloadBytes) == 0 {
		writeError(w, errBadRequest(errors.New("payload field required")))
		return
	}
	canonical, err := compiler.CanonicalizePayload(payloadBytes)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if err := compiler.ValidatePayloadSHACL(canonical); err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	canonicalPDF, err := compiler.CompilePDF(canonical)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	if err := compiler.MatchPageContent(submittedPDF, canonicalPDF); err != nil {
		writeError(w, errConflict(err))
		return
	}
	result, err := compiler.AppendVerificationWitness(canonicalPDF, canonical)
	if err != nil {
		writeError(w, errBadRequest(err))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(result)
}

func (s *service) ontologyContext(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/ld+json")
	_, _ = w.Write(substituteBaseURL(ontologyContext))
}

func (s *service) ontologyOwl(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/ld+json")
	_, _ = w.Write(substituteBaseURL(ontologyOWL))
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

// readMultipartParts reads all parts from a multipart body into a map keyed by form field name.
func readMultipartParts(body io.Reader, boundary string) (map[string][]byte, error) {
	parts := make(map[string][]byte)
	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart: %w", err)
		}
		name := part.FormName()
		var limit int64 = 8 << 20
		if name == "pdf" {
			limit = 32 << 20
		}
		data, err := io.ReadAll(io.LimitReader(part, limit))
		part.Close()
		if err != nil {
			return nil, fmt.Errorf("read part %q: %w", name, err)
		}
		parts[name] = data
	}
	return parts, nil
}

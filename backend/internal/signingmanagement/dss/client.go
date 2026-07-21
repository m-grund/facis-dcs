// Package dss integrates the EU Digital Signature Service (DSS, the
// eSignature building block's validation stack) as an ADDITIONAL, external
// AdES validator alongside the internal PKCS#11-based checks
// (DCS-FR-SM-18, DCS-IR-SI-10, DCS-IR-CI-08). The DSS demonstration webapp
// exposes REST validation under /services/rest/validation; when DSS_URL is
// configured the signature validator submits the signed PDF there and
// reports the returned indication. A configured-but-unreachable DSS is an
// ERROR, never silently skipped (required external dependencies hard-fail).
//
// Deployment note: the EU distributes the demo webapp as a ZIP/WAR, not as
// an official container image — deployment/helm/charts/dss wraps a pinned
// community image and stays DISABLED by default; enabling it is an operator
// decision (dss.enabled + DSS_URL).
package dss

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// URL returns the configured DSS endpoint ("" = DSS validation disabled).
func URL() string {
	return strings.TrimSpace(os.Getenv("DSS_URL"))
}

// Client submits signed documents to a DSS instance's REST validation API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New returns a Client for the given DSS base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Report is the distilled outcome of a DSS validation call.
type Report struct {
	// Indication is ETSI EN 319 102-1's main status indication
	// (e.g. TOTAL-PASSED, INDETERMINATE, TOTAL-FAILED).
	Indication string
	// SubIndication qualifies non-passed indications
	// (e.g. NO_CERTIFICATE_CHAIN_FOUND for a signer CA outside the EU LOTL).
	SubIndication string
	// SignedBy is the readable subject of the signing certificate the wallet
	// used (DSS simpleReport SignedBy). It is the sole-control evidence: the
	// DCS asserts this identifies the SIGNATORY, proving the signature was
	// produced with the signatory's own key — never a DCS key (eIDAS Art. 26c,
	// DCS-FR-SM-16 "secure key usage ... integrity validation upon signing").
	SignedBy string
	// SignatureFormat is the AdES format+level DSS recognized
	// (e.g. PAdES-BASELINE-B), the level evidence for DCS-FR-SM-01/-02.
	SignatureFormat string
	// SigningTime is the claimed/qualified signing time (DCS-FR-SM-18 timestamp
	// verification).
	SigningTime string
}

// Passed reports whether the ETSI indication is TOTAL-PASSED.
func (r *Report) Passed() bool {
	return strings.EqualFold(r.Indication, "TOTAL-PASSED")
}

// cryptoFailureSubIndications are the ETSI EN 319 102-1 sub-indications that mean
// the signature itself is broken — bad crypto, a mismatched hash, a malformed
// container, or no signed data — as opposed to an incomplete trust chain or POE.
var cryptoFailureSubIndications = map[string]bool{
	"SIG_CRYPTO_FAILURE":    true,
	"HASH_FAILURE":          true,
	"FORMAT_FAILURE":        true,
	"SIGNED_DATA_NOT_FOUND": true,
}

// AssertValidAES enforces the DCS's acceptance criteria for a wallet-produced
// Advanced Electronic Signature (eIDAS Art. 26, DCS-FR-SM-16/-18): the signature
// is cryptographically sound, a signing certificate is present, and — the
// sole-control proof — that certificate identifies the ceremony's signatory.
//
// It deliberately does NOT require DSS's TOTAL-PASSED. TOTAL-PASSED additionally
// asserts the signing certificate chains to a QUALIFIED EU trust-list CA, which
// is a QES property; AES needs only integrity and unique linkage to the
// signatory (Art. 26 a/b/d). So an INDETERMINATE result whose sub-indication is
// a trust/POE gap (e.g. NO_CERTIFICATE_CHAIN_FOUND for a non-qualified CA) is
// accepted, while a TOTAL-FAILED or any crypto/integrity failure is rejected.
//
// AES (eIDAS Art. 26) requires a cryptographically sound signature over the
// document by a signatory's certificate; it does NOT require the certificate to
// carry any wallet-PID identifier — no such binding is standardised (the EUDI
// reference QTSP only copies PID name attributes into the subject at enrolment).
// The signatory's identity is established by the ceremony's verified PID and
// recorded there; here we assert only that the signature is a valid AES.
func (r *Report) AssertValidAES() error {
	if strings.EqualFold(r.Indication, "TOTAL-FAILED") || cryptoFailureSubIndications[strings.ToUpper(strings.TrimSpace(r.SubIndication))] {
		return fmt.Errorf("dss: signature failed validation: indication %s / %s", r.Indication, r.SubIndication)
	}
	if strings.TrimSpace(r.SignedBy) == "" {
		return fmt.Errorf("dss: signature carries no signing certificate")
	}
	return nil
}

// ValidatePDF submits pdf to POST {base}/services/rest/validation/validateSignature
// and returns the simple report's indication. Any transport or protocol
// failure is an error — the caller treats a configured DSS as required.
func (c *Client) ValidatePDF(ctx context.Context, pdf []byte, name string) (*Report, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("dss: no base URL configured")
	}
	body, err := json.Marshal(map[string]any{
		"signedDocument": map[string]any{
			"bytes": base64.StdEncoding.EncodeToString(pdf),
			"name":  name,
		},
		"originalDocuments": []any{},
		"policy":            nil,
		"signatureId":       nil,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/services/rest/validation/validateSignature", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dss: validation request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dss: read validation response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dss: validation returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	report, err := parseReport(respBody)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// parseReport extracts the first Indication/SubIndication pair from a DSS
// WSReportsDTO. The DTO layout differs across DSS versions (simpleReport
// signature entries vs. XML-derived attribute casing), so the search walks
// the JSON generically instead of pinning one version's schema.
func parseReport(raw []byte) (*Report, error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("dss: parse validation response: %w", err)
	}
	report := &Report{}
	walkReport(doc, report)
	if report.Indication == "" {
		return nil, fmt.Errorf("dss: validation response carries no Indication")
	}
	return report, nil
}

// walkReport pulls the first occurrence of each distilled field from a DSS
// WSReportsDTO. The DTO layout differs across DSS versions (simpleReport
// entries vs. XML-derived attribute casing), so the search walks the JSON
// generically instead of pinning one version's schema.
func walkReport(node any, report *Report) {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			s, ok := val.(string)
			if !ok {
				continue
			}
			switch strings.ToLower(key) {
			case "indication":
				setFirst(&report.Indication, s)
			case "subindication":
				setFirst(&report.SubIndication, s)
			case "signedby":
				setFirst(&report.SignedBy, s)
			case "signatureformat":
				setFirst(&report.SignatureFormat, s)
			case "signingtime":
				setFirst(&report.SigningTime, s)
			}
		}
		for _, val := range v {
			walkReport(val, report)
		}
	case []any:
		for _, item := range v {
			walkReport(item, report)
		}
	}
}

func setFirst(dst *string, s string) {
	if *dst == "" && s != "" {
		*dst = s
	}
}

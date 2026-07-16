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
	findIndications(doc, report)
	if report.Indication == "" {
		return nil, fmt.Errorf("dss: validation response carries no Indication")
	}
	return report, nil
}

func findIndications(node any, report *Report) {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			lower := strings.ToLower(key)
			if s, ok := val.(string); ok {
				if lower == "indication" && report.Indication == "" {
					report.Indication = s
				}
				if lower == "subindication" && report.SubIndication == "" {
					report.SubIndication = s
				}
			}
		}
		for _, val := range v {
			findIndications(val, report)
		}
	case []any:
		for _, item := range v {
			findIndications(item, report)
		}
	}
}

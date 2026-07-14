package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ContractTargetClient dispatches a deployment payload to the configured
// Contract Target System (UC-05-01). Implementations are best-effort: the
// deploy command persists the deployment record and returns its response
// regardless of whether the outbound call below succeeds, since the target
// system's own acknowledgement (POST /contract/deployment/callback) is the
// authoritative signal of a successful deployment (DCS-FR-SM-12).
type ContractTargetClient interface {
	Deploy(ctx context.Context, payload map[string]any) error
}

// HTTPContractTargetClient POSTs the deployment payload to a configured URL
// (deployment/helm/charts/orce/flows/contract-target-flow.json is the
// reference implementation of the receiving side).
type HTTPContractTargetClient struct {
	url        string
	httpClient *http.Client
}

// ContractTargetURL returns the configured Contract Target System endpoint,
// or "" if none is configured.
func ContractTargetURL() string {
	return strings.TrimSpace(os.Getenv("CONTRACT_TARGET_URL"))
}

func NewHTTPContractTargetClient(url string) *HTTPContractTargetClient {
	return &HTTPContractTargetClient{
		url:        strings.TrimSpace(url),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *HTTPContractTargetClient) Deploy(ctx context.Context, payload map[string]any) error {
	if c == nil || c.url == "" {
		return fmt.Errorf("contract target URL is empty")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal deployment payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create deployment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post deployment payload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("contract target returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

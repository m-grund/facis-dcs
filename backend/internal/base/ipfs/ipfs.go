package ipfs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type APIClient struct {
	baseURL    string
	mfsBaseURL string
	client     *http.Client
}

func NewClient(baseURL string, mfsBaseURL string) *APIClient {
	return &APIClient{
		baseURL:    baseURL,
		mfsBaseURL: mfsBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type IPFSResult struct {
	Identifier struct {
		Format string `json:"Format"`
		Value  string `json:"Value"`
	} `json:"identifier"`
	Data json.RawMessage `json:"data"`
}

func (c *APIClient) CreateFile(ctx context.Context, data any) (*IPFSResult, error) {

	url := c.baseURL + "/api/ipfs/create"

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result IPFSResult
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if c.mfsBaseURL != "" {
		err := c.copyToMFS(ctx, c.mfsBaseURL, result.Identifier.Value, result.Identifier.Value)
		if err != nil {
			return &result, err
		}
	}

	return &result, nil
}

func (c *APIClient) FetchFile(cid string) (*IPFSResult, error) {
	url := fmt.Sprintf("%s/api/ipfs/%s", c.baseURL, cid)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result IPFSResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Data) > 0 {
		var dataStr string
		if err := json.Unmarshal(result.Data, &dataStr); err == nil {
			decoded, err := base64.RawStdEncoding.DecodeString(dataStr)
			if err != nil {
				decoded, _ = base64.StdEncoding.DecodeString(dataStr)
			}
			result.Data = json.RawMessage(decoded)
		}
	}

	return &result, nil
}

func (c *APIClient) DeleteFile(cid string) error {

	url := fmt.Sprintf("%s/api/ipfs/%s", c.baseURL, cid)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil

}

func (c *APIClient) copyToMFS(ctx context.Context, baseURL string, cid string, filename string) error {

	url := fmt.Sprintf("%s/api/v0/files/cp?arg=/ipfs/%s&arg=/%s", baseURL, cid, filename)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

package ipfs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
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
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}

	if c.baseURL == "" {
		return c.createKuboFile(ctx, jsonData)
	}

	url := c.baseURL + "/api/ipfs/create"
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
	if c.baseURL == "" {
		return c.fetchKuboFile(cid)
	}

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
	if c.baseURL == "" {
		return c.deleteKuboFile(cid)
	}

	url := fmt.Sprintf("%s/api/ipfs/%s", c.baseURL, cid)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil

}

func (c *APIClient) createKuboFile(ctx context.Context, data []byte) (*IPFSResult, error) {
	if c.mfsBaseURL == "" {
		return nil, fmt.Errorf("IPFS_MFS_BASE_URL is required when IPFS_TENANT_BASE_URL is not configured")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="audit-log.json"`)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("write multipart data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	url := c.mfsBaseURL + "/api/v0/add?pin=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("create Kubo add request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do Kubo add request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected Kubo add status %d: %s", resp.StatusCode, body)
	}

	var addResult struct {
		Hash string `json:"Hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&addResult); err != nil {
		return nil, fmt.Errorf("decode Kubo add response: %w", err)
	}
	if addResult.Hash == "" {
		return nil, fmt.Errorf("Kubo add response did not include a CID")
	}

	result := &IPFSResult{
		Data: data,
	}
	result.Identifier.Format = "CID"
	result.Identifier.Value = addResult.Hash

	if err := c.copyToMFS(ctx, c.mfsBaseURL, addResult.Hash, addResult.Hash); err != nil {
		return result, err
	}

	return result, nil
}

func (c *APIClient) fetchKuboFile(cid string) (*IPFSResult, error) {
	if c.mfsBaseURL == "" {
		return nil, fmt.Errorf("IPFS_MFS_BASE_URL is required when IPFS_TENANT_BASE_URL is not configured")
	}

	url := fmt.Sprintf("%s/api/v0/cat?arg=%s", c.mfsBaseURL, cid)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create Kubo cat request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do Kubo cat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected Kubo cat status %d: %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Kubo cat response: %w", err)
	}

	result := &IPFSResult{
		Data: body,
	}
	result.Identifier.Format = "CID"
	result.Identifier.Value = cid

	return result, nil
}

func (c *APIClient) deleteKuboFile(cid string) error {
	if c.mfsBaseURL == "" {
		return fmt.Errorf("IPFS_MFS_BASE_URL is required when IPFS_TENANT_BASE_URL is not configured")
	}

	url := fmt.Sprintf("%s/api/v0/pin/rm?arg=%s", c.mfsBaseURL, cid)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create Kubo unpin request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("do Kubo unpin request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected Kubo unpin status %d: %s", resp.StatusCode, body)
	}

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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected Kubo files/cp status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// Package hydra implements OAuth2/OIDC and Hydra admin interactions for DCS login.
package hydra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// Config holds Hydra issuer and DCS OAuth client settings.
type Config struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AdminURL     string
}

// Client talks to Hydra public OIDC endpoints and the admin API.
type Client struct {
	cfg        Config
	metadataMu sync.RWMutex
	metadata   *ProviderMetadata
}

// ProviderMetadata is the subset of OpenID discovery used by DCS.
type ProviderMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// TokenResponse is a Hydra token endpoint payload.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// New builds a Client from explicit configuration.
func New(cfg Config) *Client {
	cfg.IssuerURL = strings.TrimRight(strings.TrimSpace(cfg.IssuerURL), "/")
	if cfg.AdminURL == "" {
		cfg.AdminURL = strings.TrimRight(strings.TrimSpace(os.Getenv("HYDRA_ADMIN_URL")), "/")
	}
	if cfg.AdminURL == "" {
		cfg.AdminURL = "http://localhost:30085"
	}
	return &Client{cfg: cfg}
}

// NewFromEnv constructs a Client using HYDRA_* environment variables.
func NewFromEnv() *Client {
	return New(Config{
		IssuerURL:    os.Getenv("HYDRA_ISSUER_URL"),
		ClientID:     os.Getenv("HYDRA_CLIENT_ID"),
		ClientSecret: os.Getenv("HYDRA_CLIENT_SECRET"),
		RedirectURI:  os.Getenv("HYDRA_REDIRECT_URI"),
	})
}

// ClientID returns the OAuth client identifier.
func (c *Client) ClientID() string {
	return c.cfg.ClientID
}

// RedirectURI returns the configured OAuth2 redirect URI for this client.
func (c *Client) RedirectURI() string {
	return c.cfg.RedirectURI
}

// ProviderMetadata loads and caches OpenID discovery for the Hydra issuer.
func (c *Client) ProviderMetadata(ctx context.Context) (*ProviderMetadata, error) {
	c.metadataMu.RLock()
	if c.metadata != nil {
		cached := *c.metadata
		c.metadataMu.RUnlock()
		return &cached, nil
	}
	c.metadataMu.RUnlock()

	wellKnown := c.cfg.IssuerURL + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openid discovery status %d", resp.StatusCode)
	}

	var metadata ProviderMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, err
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" {
		return nil, fmt.Errorf("openid discovery missing authorization or token endpoint")
	}

	c.metadataMu.Lock()
	c.metadata = &metadata
	c.metadataMu.Unlock()
	return &metadata, nil
}

// ExchangeCode exchanges an authorization code for tokens.
func (c *Client) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	metadata, err := c.ProviderMetadata(ctx)
	if err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", c.cfg.ClientID)
	if c.cfg.ClientSecret != "" {
		data.Set("client_secret", c.cfg.ClientSecret)
	}
	data.Set("redirect_uri", c.cfg.RedirectURI)

	return c.postTokenEndpoint(ctx, metadata.TokenEndpoint, data)
}

// RefreshAccessToken obtains a new access token from a refresh token.
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	metadata, err := c.ProviderMetadata(ctx)
	if err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", c.cfg.ClientID)
	if c.cfg.ClientSecret != "" {
		data.Set("client_secret", c.cfg.ClientSecret)
	}

	return c.postTokenEndpoint(ctx, metadata.TokenEndpoint, data)
}

// RevokeRefreshToken revokes a refresh token at the provider revocation endpoint.
func (c *Client) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	metadata, err := c.ProviderMetadata(ctx)
	if err != nil || metadata.RevocationEndpoint == "" {
		return err
	}

	data := url.Values{}
	data.Set("token", refreshToken)
	data.Set("client_id", c.cfg.ClientID)
	if c.cfg.ClientSecret != "" {
		data.Set("client_secret", c.cfg.ClientSecret)
	}
	data.Set("token_type_hint", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.RevocationEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("revoke failed with status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) postTokenEndpoint(ctx context.Context, tokenEndpoint string, data url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	return &tokenResp, nil
}

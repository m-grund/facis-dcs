package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	genauth "digital-contracting-service/gen/auth"
	"digital-contracting-service/internal/pathutil"

	"goa.design/clue/log"
	goa "goa.design/goa/v3/pkg"
)

// authSvc implements the generated auth.Service interface.
type authSvc struct {
	oidcIssuerURL     string
	oidcClientID      string
	redirectURI       string
	logoutRedirectURI string
	uiBasePath        string
}

// NewAuth returns the Auth service implementation.
func NewAuth() genauth.Service {
	return &authSvc{
		oidcIssuerURL:     os.Getenv("OIDC_ISSUER_URL"),
		oidcClientID:      os.Getenv("OIDC_CLIENT_ID"),
		redirectURI:       os.Getenv("OIDC_REDIRECT_URI"),
		logoutRedirectURI: os.Getenv("OIDC_LOGOUT_REDIRECT_URI"),
		uiBasePath:        pathutil.NormalizePath(os.Getenv("DCS_UI_PATH"), "/ui/", true),
	}
}

// Login returns the Keycloak OIDC authorization URL.
func (s *authSvc) Login(ctx context.Context) (*genauth.LoginResult, error) {
	log.Printf(ctx, "auth.login")
	params := url.Values{}
	params.Set("client_id", s.oidcClientID)
	params.Set("redirect_uri", s.redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "openid")
	authURL := s.oidcIssuerURL + "/protocol/openid-connect/auth?" + params.Encode()
	return &genauth.LoginResult{AuthURL: authURL}, nil
}

// keycloakTokenResponse is the raw response from Keycloak's token endpoint.
type keycloakTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// Callback exchanges the authorization code for tokens.
// The refresh_token cookie is set by SetRefreshTokenInContext.
// After setting the cookie, it redirects to /auth/success.
func (s *authSvc) Callback(ctx context.Context, p *genauth.CallbackPayload) (*genauth.CallbackResult, error) {
	log.Printf(ctx, "auth.callback")

	tokenResp, err := s.exchangeCodeForToken(ctx, p.Code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// Stash the refresh token in context so the response encoder can set the cookie.
	// This is picked up by SetRefreshTokenInContext which sets the cookie immediately.
	SetRefreshTokenInContext(ctx, tokenResp.RefreshToken)

	// Redirect to frontend auth success route under configured UI base path.
	return &genauth.CallbackResult{
		Location: s.uiBasePath + "auth/success",
	}, nil
}

// Refresh exchanges the refresh_token (from HttpOnly cookie) for a new access token.
func (s *authSvc) Refresh(ctx context.Context) (*genauth.RefreshResult, error) {
	log.Printf(ctx, "auth.refresh")

	// Extract *http.Request from context (injected by RequestContextMiddleware).
	r, ok := HTTPRequestFromContext(ctx)
	if !ok {
		return nil, goa.PermanentError("unauthorized", "missing HTTP request in context")
	}

	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		return nil, goa.PermanentError("unauthorized", "missing or invalid refresh token")
	}

	tokenResp, err := s.refreshAccessToken(ctx, cookie.Value)
	if err != nil {
		return nil, goa.PermanentError("unauthorized", "token refresh failed: %v", err)
	}

	return &genauth.RefreshResult{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		ExpiresIn:   tokenResp.ExpiresIn,
	}, nil
}

// Logout returns the Keycloak OIDC logout URL.
func (s *authSvc) Logout(ctx context.Context) (*genauth.LogoutResult, error) {
	log.Printf(ctx, "auth.logout")

	// Build Keycloak logout URL with configured post-logout redirect
	postLogoutRedirect := s.uiBasePath
	if s.logoutRedirectURI != "" {
		postLogoutRedirect = s.logoutRedirectURI
	}

	params := url.Values{}
	params.Set("client_id", s.oidcClientID)
	params.Set("post_logout_redirect_uri", postLogoutRedirect)
	logoutURL := s.oidcIssuerURL + "/protocol/openid-connect/logout?" + params.Encode()

	return &genauth.LogoutResult{
		LogoutURL: logoutURL,
	}, nil
}

// exchangeCodeForToken POSTs the auth code to Keycloak's token endpoint.
func (s *authSvc) exchangeCodeForToken(ctx context.Context, code string) (*keycloakTokenResponse, error) {
	tokenEndpoint := s.oidcIssuerURL + "/protocol/openid-connect/token"
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", s.oidcClientID)
	data.Set("redirect_uri", s.redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf(ctx, "Failed to close response body: %v", err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp keycloakTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	return &tokenResp, nil
}

// refreshAccessToken asks Keycloak for a new access token using the refresh token.
func (s *authSvc) refreshAccessToken(ctx context.Context, refreshToken string) (*keycloakTokenResponse, error) {
	tokenEndpoint := s.oidcIssuerURL + "/protocol/openid-connect/token"
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", s.oidcClientID)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf(ctx, "Failed to close response body: %v", err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp keycloakTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	return &tokenResp, nil
}

// revokeToken revokes a refresh token with Keycloak.
func (s *authSvc) revokeToken(ctx context.Context, refreshToken string) error {
	revokeEndpoint := s.oidcIssuerURL + "/protocol/openid-connect/revoke"
	data := url.Values{}
	data.Set("token", refreshToken)
	data.Set("client_id", s.oidcClientID)
	data.Set("token_type_hint", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", revokeEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf(ctx, "Failed to close response body: %v", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("revoke failed with status %d", resp.StatusCode)
	}
	return nil
}

// LogoutComplete finalizes logout by revoking the refresh token and clearing the cookie.
// This endpoint is called by Keycloak after the user confirms logout.
func (s *authSvc) LogoutComplete(ctx context.Context) (*genauth.LogoutCompleteResult, error) {
	log.Printf(ctx, "auth.logout-complete")

	// Extract *http.Request from context
	r, ok := HTTPRequestFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("missing HTTP request in context")
	}

	// Try to get and revoke the refresh token
	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie.Value != "" {
		// Best effort: revoke the token with Keycloak
		_ = s.revokeToken(ctx, cookie.Value)
	}

	// Clear the refresh token cookie
	ClearRefreshTokenCookie(ctx)

	// Redirect to frontend UI under configured base path
	return &genauth.LogoutCompleteResult{
		Location: s.uiBasePath,
	}, nil
}

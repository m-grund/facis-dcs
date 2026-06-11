package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"

	genauth "digital-contracting-service/gen/auth"
	authdb "digital-contracting-service/internal/auth/db"
	"digital-contracting-service/internal/auth/hydra"
	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/pathutil"

	"goa.design/clue/log"
	goa "goa.design/goa/v3/pkg"
)

const oauthStateSizeBytes = 24

type authSvc struct {
	hydra             *hydra.Client
	logoutRedirectURI string
	uiBasePath        string
	publicAPIBase     string
	presentations     authdb.PresentationAttemptRepo
	requestSigner     oid4vp.AuthorizationRequestSigner
}

func NewAuth(presentations authdb.PresentationAttemptRepo) genauth.Service {
	var requestSigner oid4vp.AuthorizationRequestSigner
	if signer, err := oid4vp.LoadAuthorizationRequestSignerFromEnv(); err == nil {
		requestSigner = signer
	} else if strings.TrimSpace(os.Getenv("VAULT_ADDR")) != "" {
		log.Printf(context.Background(), "oid4vp request signer not loaded: %v", err)
	}

	return &authSvc{
		hydra:             hydra.NewFromEnv(),
		logoutRedirectURI: os.Getenv("HYDRA_POST_LOGOUT_REDIRECT_URI"),
		uiBasePath:        pathutil.NormalizePath(os.Getenv("DCS_UI_PATH"), "/ui/", true),
		publicAPIBase:     publicAPIBaseURL(),
		presentations:     presentations,
		requestSigner:     requestSigner,
	}
}

func (s *authSvc) Callback(ctx context.Context, p *genauth.CallbackPayload) (*genauth.CallbackResult, error) {
	log.Printf(ctx, "auth.callback")
	defer ClearOAuthStateCookie(ctx)

	if oauthErr := oauthCallbackError(p); oauthErr != "" {
		clearAuthSessionCookies(ctx)
		log.Printf(ctx, "auth.callback oauth error: %s", oauthErr)
		return &genauth.CallbackResult{Location: oauthErrorRedirectLocation(s.uiBasePath, p)}, nil
	}

	code := ""
	if p.Code != nil {
		code = strings.TrimSpace(*p.Code)
	}

	if code == "" {
		return nil, goa.PermanentError("bad_request", "missing authorization code")
	}

	returnedState := ""
	if p.State != nil {
		returnedState = *p.State
	}

	if err := validateOAuthState(ctx, returnedState); err != nil {
		return nil, goa.PermanentError("unauthorized", "invalid oauth state: %v", err)
	}

	tokenResp, err := s.hydra.ExchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	if strings.TrimSpace(tokenResp.RefreshToken) == "" {
		return nil, goa.PermanentError("unauthorized", "token response missing refresh_token")
	}

	if strings.TrimSpace(tokenResp.IDToken) == "" {
		return nil, goa.PermanentError("unauthorized", "token response missing id_token (openid scope required)")
	}

	SetRefreshTokenInContext(ctx, tokenResp.RefreshToken)
	SetIDTokenCookie(ctx, tokenResp.IDToken)

	return &genauth.CallbackResult{
		Location: s.uiBasePath + "auth/success",
	}, nil
}

func (s *authSvc) Refresh(ctx context.Context) (*genauth.RefreshResult, error) {
	log.Printf(ctx, "auth.refresh")

	r, ok := HTTPRequestFromContext(ctx)
	if !ok {
		return nil, goa.PermanentError("unauthorized", "missing HTTP request in context")
	}

	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		return nil, goa.PermanentError("unauthorized", "missing or invalid refresh token")
	}

	tokenResp, err := s.hydra.RefreshAccessToken(ctx, cookie.Value)
	if err != nil {
		ClearRefreshTokenCookie(ctx)
		return nil, goa.PermanentError("unauthorized", "token refresh failed: %v", err)
	}

	if strings.TrimSpace(tokenResp.RefreshToken) != "" {
		SetRefreshTokenInContext(ctx, tokenResp.RefreshToken)
	}

	if strings.TrimSpace(tokenResp.IDToken) != "" {
		SetIDTokenCookie(ctx, tokenResp.IDToken)
	}

	return &genauth.RefreshResult{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		ExpiresIn:   tokenResp.ExpiresIn,
	}, nil
}

func (s *authSvc) Logout(ctx context.Context) (*genauth.LogoutResult, error) {
	log.Printf(ctx, "auth.logout")

	if strings.TrimSpace(s.logoutRedirectURI) == "" {
		return nil, goa.PermanentError("unauthorized", "HYDRA_POST_LOGOUT_REDIRECT_URI is not configured")
	}

	idTokenHint := readIDTokenCookie(ctx)
	if idTokenHint == "" {
		return nil, goa.PermanentError("unauthorized", "missing id_token session cookie")
	}

	if r, ok := HTTPRequestFromContext(ctx); ok {
		if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
			_ = s.hydra.RevokeRefreshToken(ctx, cookie.Value)
		}
	}

	metadata, err := s.hydra.ProviderMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("hydra openid discovery failed: %w", err)
	}

	if metadata.EndSessionEndpoint == "" {
		return nil, goa.PermanentError("unauthorized", "Hydra provider missing end_session_endpoint")
	}

	params := url.Values{}
	params.Set("client_id", s.hydra.ClientID())
	params.Set("id_token_hint", idTokenHint)
	params.Set("post_logout_redirect_uri", s.logoutRedirectURI)
	logoutURL := metadata.EndSessionEndpoint + "?" + params.Encode()

	ClearRefreshTokenCookie(ctx)
	ClearIDTokenCookie(ctx)
	return &genauth.LogoutResult{LogoutURL: logoutURL}, nil
}

func (s *authSvc) LogoutComplete(ctx context.Context) (*genauth.LogoutCompleteResult, error) {
	log.Printf(ctx, "auth.logout-complete")
	ClearRefreshTokenCookie(ctx)
	ClearIDTokenCookie(ctx)
	return &genauth.LogoutCompleteResult{Location: s.uiBasePath}, nil
}

func newOAuthState() (string, error) {
	buf := make([]byte, oauthStateSizeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func oauthCallbackError(p *genauth.CallbackPayload) string {
	if p.Error == nil {
		return ""
	}
	return strings.TrimSpace(*p.Error)
}

func oauthErrorRedirectLocation(uiBasePath string, p *genauth.CallbackPayload) string {
	loc, err := url.Parse(uiBasePath)
	if err != nil {
		return uiBasePath
	}

	q := loc.Query()
	if errCode := oauthCallbackError(p); errCode != "" {
		q.Set("auth_error", errCode)
	}

	if p.ErrorDescription != nil {
		if desc := strings.TrimSpace(*p.ErrorDescription); desc != "" {
			q.Set("auth_error_description", desc)
		}
	}

	loc.RawQuery = q.Encode()

	return loc.String()
}

func clearAuthSessionCookies(ctx context.Context) {
	ClearRefreshTokenCookie(ctx)
	ClearIDTokenCookie(ctx)
}

func validateOAuthState(ctx context.Context, returnedState string) error {
	if returnedState == "" {
		return fmt.Errorf("missing state")
	}

	expected, err := ReadOAuthStateCookie(ctx)
	if err != nil {
		return err
	}

	if subtle.ConstantTimeCompare([]byte(expected), []byte(returnedState)) == 1 {
		return nil
	}

	return fmt.Errorf("state mismatch")
}

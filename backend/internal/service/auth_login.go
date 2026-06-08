package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	genauth "digital-contracting-service/gen/auth"
	authdb "digital-contracting-service/internal/auth/db"
	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/pathutil"

	goa "goa.design/goa/v3/pkg"
)

const defaultOID4VPStateTTL = 5 * time.Minute

func (s *authSvc) LoginRenew(ctx context.Context, p *genauth.LoginRenewPayload) (*genauth.LoginRenewResult, error) {
	state := strings.TrimSpace(p.State)
	if state == "" {
		return nil, goa.PermanentError("bad_request", "state is required")
	}

	attempt, err := s.presentations.FindByPresentationState(ctx, state)
	if err != nil {
		return nil, err
	}
	if attempt == nil {
		return nil, goa.PermanentError("not_found", "unknown presentation state")
	}
	switch attempt.Status {
	case authdb.PresentationPending, authdb.PresentationExpired:
	default:
		return nil, goa.PermanentError("bad_request", "presentation state cannot be renewed")
	}

	ttl := oid4vpStateTTL()
	expiresAt := time.Now().UTC().Add(ttl)
	if err := s.presentations.RenewPending(ctx, state, expiresAt); err != nil {
		if authdb.IsPresentationNotPending(err) {
			return nil, goa.PermanentError("bad_request", "presentation state cannot be renewed")
		}
		return nil, err
	}

	login, err := s.buildLoginResult(ctx, state, attempt.Nonce, int(ttl.Seconds()))
	if err != nil {
		return nil, err
	}
	return loginResultToRenew(login), nil
}

func loginResultToRenew(in *genauth.LoginResult) *genauth.LoginRenewResult {
	return &genauth.LoginRenewResult{
		RequestURI:      in.RequestURI,
		PresentationURL: in.PresentationURL,
		State:           in.State,
		AuthorizeURL:    in.AuthorizeURL,
		ExpiresIn:       in.ExpiresIn,
	}
}

func (s *authSvc) Login(ctx context.Context) (*genauth.LoginResult, error) {
	ttl := oid4vpStateTTL()
	presentationState, err := newOAuthState()
	if err != nil {
		return nil, err
	}
	nonce, err := newOAuthState()
	if err != nil {
		return nil, err
	}

	SetOAuthStateCookie(ctx, presentationState)

	expiresAt := time.Now().UTC().Add(ttl)
	if err := s.presentations.CreatePending(ctx, authdb.PresentationAttempt{
		PresentationState: presentationState,
		Nonce:             nonce,
		ExpiresAt:         expiresAt,
	}); err != nil {
		return nil, err
	}

	return s.buildLoginResult(ctx, presentationState, nonce, int(ttl.Seconds()))
}

func (s *authSvc) buildLoginResult(ctx context.Context, presentationState, nonce string, expiresIn int) (*genauth.LoginResult, error) {
	authorizeURL, err := s.hydra.AuthorizeURL(ctx, presentationState)
	if err != nil {
		return nil, fmt.Errorf("hydra authorize url: %w", err)
	}
	requestURI := strings.TrimRight(s.publicAPIBase, "/") + "/auth/presentation/request/" + url.PathEscape(presentationState)
	return &genauth.LoginResult{
		RequestURI:      requestURI,
		PresentationURL: buildOpenID4VPPresentationURI(s.hydra.ClientID(), requestURI),
		State:           presentationState,
		AuthorizeURL:    authorizeURL,
		ExpiresIn:       expiresIn,
	}, nil
}

func (s *authSvc) Consent(ctx context.Context, p *genauth.ConsentPayload) (*genauth.ConsentResult, error) {
	challenge := strings.TrimSpace(p.ConsentChallenge)
	if challenge == "" {
		return nil, goa.PermanentError("bad_request", "consent_challenge is required")
	}

	redirectTo, err := s.hydra.AcceptConsent(ctx, challenge, "", nil)
	if err != nil {
		return nil, goa.PermanentError("unauthorized", "hydra consent: %v", err)
	}
	redirectTo, err = s.hydra.ResolveRedirectChain(ctx, redirectTo, "", nil)
	if err != nil {
		return nil, goa.PermanentError("unauthorized", "hydra consent redirect: %v", err)
	}

	location := normalizeBrowserContinueURL(s.hydra.RedirectURI(), redirectTo)
	return &genauth.ConsentResult{Location: location}, nil
}

func (s *authSvc) LoginChallenge(ctx context.Context, p *genauth.LoginChallengePayload) error {
	challenge := strings.TrimSpace(p.LoginChallenge)
	if challenge == "" {
		return goa.PermanentError("bad_request", "login_challenge is required")
	}

	attempt, err := s.loadPresentationAttempt(ctx, p.State)
	if err != nil {
		return err
	}
	if attempt.Status != authdb.PresentationPending {
		return goa.PermanentError("bad_request", "presentation state is not pending")
	}
	if attempt.HydraLoginChallenge != nil && strings.TrimSpace(*attempt.HydraLoginChallenge) == challenge {
		return nil
	}

	if err := s.presentations.SetHydraLoginChallenge(ctx, p.State, challenge); err != nil {
		if authdb.IsPresentationNotPending(err) {
			return goa.PermanentError("bad_request", "presentation state is not pending")
		}
		return err
	}
	return nil
}

func (s *authSvc) PresentationRequest(ctx context.Context, p *genauth.PresentationRequestPayload) (*genauth.PresentationRequestObject, error) {
	attempt, err := s.loadPresentationAttempt(ctx, p.State)
	if err != nil {
		return nil, err
	}
	dcql, err := oid4vp.LoadDCQLQuery()
	if err != nil {
		return nil, err
	}
	responseURI := strings.TrimRight(s.publicAPIBase, "/") + "/auth/presentation/callback"
	return &genauth.PresentationRequestObject{
		ClientID:     s.hydra.ClientID(),
		ResponseType: "vp_token",
		ResponseMode: "direct_post",
		ResponseURI:  responseURI,
		State:        attempt.PresentationState,
		Nonce:        attempt.Nonce,
		DcqlQuery:    dcql,
	}, nil
}

func (s *authSvc) PresentationCallback(ctx context.Context, p *genauth.PresentationCallbackPayload) (*genauth.PresentationCallbackResult, error) {
	attempt, err := s.loadPresentationAttempt(ctx, p.State)
	if err != nil {
		return nil, err
	}
	if attempt.Status != authdb.PresentationPending {
		return nil, goa.PermanentError("bad_request", "presentation state is not pending")
	}
	if attempt.HydraLoginChallenge == nil || strings.TrimSpace(*attempt.HydraLoginChallenge) == "" {
		return nil, goa.PermanentError("bad_request", "missing hydra login challenge; complete browser authorize first")
	}

	vpToken := ""
	if p.VpToken != nil {
		vpToken = *p.VpToken
	}
	cfg, err := oid4vp.LoadTrustConfigFromEnv()
	var verifier oid4vp.Verifier
	if err != nil {
		verifier = s.vpVerifier
	} else {
		verifier = oid4vp.NewVerifier(cfg)
	}
	verified, err := verifier.Verify(vpToken, oid4vp.PresentationContext{
		Nonce:    attempt.Nonce,
		ClientID: s.hydra.ClientID(),
	})
	if err != nil {
		_ = s.presentations.MarkFailed(ctx, attempt.PresentationState, err.Error())
		oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
			PresentationState: attempt.PresentationState,
			Success:           false,
			ErrorMessage:      err.Error(),
		})
		return nil, goa.PermanentError("unauthorized", "vp verification failed: %v", err)
	}
	if err := oid4vp.CheckCredentialRevocation(verified.RawClaims); err != nil {
		_ = s.presentations.MarkFailed(ctx, attempt.PresentationState, err.Error())
		oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
			PresentationState: attempt.PresentationState,
			Success:           false,
			SubjectDID:        verified.SubjectDID,
			ParticipantDID:    verified.ParticipantDID,
			Roles:             verified.Roles,
			ErrorMessage:      err.Error(),
		})
		return nil, goa.PermanentError("unauthorized", "credential revocation check failed: %v", err)
	}
	if err := oid4vp.CheckDisclosedClaimsMeetDCQL(verified.RawClaims); err != nil {
		_ = s.presentations.MarkFailed(ctx, attempt.PresentationState, err.Error())
		oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
			PresentationState: attempt.PresentationState,
			Success:           false,
			SubjectDID:        verified.SubjectDID,
			ParticipantDID:    verified.ParticipantDID,
			Roles:             verified.Roles,
			ErrorMessage:      err.Error(),
		})
		return nil, goa.PermanentError("unauthorized", "presentation does not meet DCQL requirements: %v", err)
	}
	grantedRoles, err := oid4vp.EvaluateLoginPolicy(verified)
	if err != nil {
		_ = s.presentations.MarkFailed(ctx, attempt.PresentationState, err.Error())
		oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
			PresentationState: attempt.PresentationState,
			Success:           false,
			SubjectDID:        verified.SubjectDID,
			ParticipantDID:    verified.ParticipantDID,
			Roles:             verified.Roles,
			ErrorMessage:      err.Error(),
		})
		return nil, goa.PermanentError("unauthorized", "login policy denied: %v", err)
	}

	redirectTo, err := s.hydra.AcceptLoginAndConsent(
		ctx,
		*attempt.HydraLoginChallenge,
		verified.SubjectDID,
		verified.ParticipantDID,
		grantedRoles,
	)
	if err != nil {
		_ = s.presentations.MarkFailed(ctx, attempt.PresentationState, err.Error())
		oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
			PresentationState: attempt.PresentationState,
			Success:           false,
			SubjectDID:        verified.SubjectDID,
			ParticipantDID:    verified.ParticipantDID,
			Roles:             grantedRoles,
			ErrorMessage:      err.Error(),
		})
		return nil, goa.PermanentError("unauthorized", "hydra login: %v", err)
	}
	continueURL := normalizeBrowserContinueURL(s.hydra.RedirectURI(), redirectTo)
	rolesJSON, _ := json.Marshal(grantedRoles)
	if err := s.presentations.MarkComplete(ctx, attempt.PresentationState, verified.RawClaims, verified.SubjectDID, verified.ParticipantDID, rolesJSON, continueURL); err != nil {
		return nil, err
	}
	oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
		PresentationState: attempt.PresentationState,
		Success:           true,
		SubjectDID:        verified.SubjectDID,
		ParticipantDID:    verified.ParticipantDID,
		Roles:             grantedRoles,
	})

	return &genauth.PresentationCallbackResult{RedirectURI: continueURL}, nil
}

func (s *authSvc) LoginStatus(ctx context.Context, p *genauth.LoginStatusPayload) (*genauth.LoginStatusResult, error) {
	attempt, err := s.presentations.FindByPresentationState(ctx, p.State)
	if err != nil {
		return nil, err
	}
	if attempt == nil {
		return nil, goa.PermanentError("not_found", "unknown presentation state")
	}

	now := time.Now().UTC()
	expiresIn := int(attempt.ExpiresAt.Sub(now).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}

	status := string(attempt.Status)
	if attempt.Status == authdb.PresentationPending && now.After(attempt.ExpiresAt) {
		if err := s.presentations.MarkExpired(ctx, attempt.PresentationState); err == nil {
			status = string(authdb.PresentationExpired)
			expiresIn = 0
		}
	}

	out := &genauth.LoginStatusResult{
		State:     attempt.PresentationState,
		Status:    status,
		ExpiresIn: expiresIn,
	}
	if attempt.RedirectURI != nil && status == string(authdb.PresentationComplete) {
		out.RedirectURI = attempt.RedirectURI
	}
	if attempt.ErrorMessage != nil && status == string(authdb.PresentationFailed) {
		out.ErrorMessage = attempt.ErrorMessage
	}
	return out, nil
}

func (s *authSvc) loadPresentationAttempt(ctx context.Context, presentationState string) (*authdb.PresentationAttempt, error) {
	attempt, err := s.presentations.FindByPresentationState(ctx, presentationState)
	if err != nil {
		return nil, err
	}
	if attempt == nil {
		return nil, goa.PermanentError("not_found", "unknown presentation state")
	}
	if attempt.Status == authdb.PresentationPending && time.Now().UTC().After(attempt.ExpiresAt) {
		_ = s.presentations.MarkExpired(ctx, presentationState)
		return nil, goa.PermanentError("bad_request", "presentation state expired")
	}
	return attempt, nil
}

// normalizeBrowserContinueURL maps Hydra redirects to the RP callback URL when code is present.
func normalizeBrowserContinueURL(configuredRedirectURI, redirectTo string) string {
	redirectTo = strings.TrimSpace(redirectTo)
	if redirectTo == "" {
		return redirectTo
	}
	u, err := url.Parse(redirectTo)
	if err != nil {
		return redirectTo
	}
	if strings.TrimSpace(u.Query().Get("code")) == "" {
		return redirectTo
	}
	configured := strings.TrimSpace(configuredRedirectURI)
	if configured == "" {
		return u.String()
	}
	cfg, err := url.Parse(configured)
	if err != nil {
		return u.String()
	}
	out := *cfg
	q := out.Query()
	for key, values := range u.Query() {
		for _, v := range values {
			q.Set(key, v)
		}
	}
	out.RawQuery = q.Encode()
	return out.String()
}

// buildOpenID4VPPresentationURI returns a cross-device wallet deep link (OpenID4VP request-by-reference).
func buildOpenID4VPPresentationURI(clientID, httpsRequestURI string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("request_uri", httpsRequestURI)
	q.Set("request_uri_method", "get")
	return "openid4vp://?" + q.Encode()
}

func oid4vpStateTTL() time.Duration {
	if v := strings.TrimSpace(os.Getenv("OID4VP_STATE_TTL_SECONDS")); v != "" {
		var secs int
		if _, err := fmt.Sscanf(v, "%d", &secs); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultOID4VPStateTTL
}

func publicAPIBaseURL() string {
	port := strings.TrimSpace(os.Getenv("DCS_HTTP_PORT"))
	if port == "" {
		port = "8991"
	}
	apiPath := pathutil.JoinPaths(apiPathPrefixEnv, defaultAPIPathPrefix, "")
	if apiPath == "" {
		apiPath = "/api"
	}
	return fmt.Sprintf("http://localhost:%s%s", port, apiPath)
}

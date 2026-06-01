package service

import (
	"context"
	"net/http"

	"digital-contracting-service/internal/pathutil"
)

const apiPathPrefixEnv = "DCS_API_PATH"
const defaultAPIPathPrefix = ""

// contextKey is a private type for context keys in this package.
type contextKey int

const (
	httpRequestKey    contextKey = iota
	refreshTokenKey   contextKey = iota
	responseWriterKey contextKey = iota
)

// RequestContextMiddleware injects the *http.Request and http.ResponseWriter
// into the context so that service implementations can access them.
// This is used by the Auth service to read cookies and set cookie headers.
func RequestContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), httpRequestKey, r)
		ctx = context.WithValue(ctx, responseWriterKey, w)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// HTTPRequestFromContext extracts the *http.Request from context.
func HTTPRequestFromContext(ctx context.Context) (*http.Request, bool) {
	r, ok := ctx.Value(httpRequestKey).(*http.Request)
	return r, ok
}

// ResponseWriterFromContext extracts the http.ResponseWriter from context.
func ResponseWriterFromContext(ctx context.Context) (http.ResponseWriter, bool) {
	w, ok := ctx.Value(responseWriterKey).(http.ResponseWriter)
	return w, ok
}

// SetRefreshTokenInContext stores the refresh token in context for the
// response encoder to pick up and set as a cookie.
// Since context is immutable, this uses the ResponseWriter to set the cookie
// header directly.
func SetRefreshTokenInContext(ctx context.Context, refreshToken string) {
	w, ok := ResponseWriterFromContext(ctx)
	if !ok || refreshToken == "" {
		return
	}
	cookiePath := pathutil.JoinPaths(apiPathPrefixEnv, defaultAPIPathPrefix, "/auth/refresh")
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     cookiePath,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	})
}

// ClearRefreshTokenCookie clears the refresh token cookie by setting MaxAge to -1.
func ClearRefreshTokenCookie(ctx context.Context) {
	w, ok := ResponseWriterFromContext(ctx)
	if !ok {
		return
	}
	cookiePath := pathutil.JoinPaths(apiPathPrefixEnv, defaultAPIPathPrefix, "/auth/refresh")
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     cookiePath,
		MaxAge:   -1, // Delete the cookie
	})
}

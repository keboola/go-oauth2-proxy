package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	middlewareapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/middleware"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	sessionsapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	internaloidc "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/providers/oidc"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/sessions/cookie"
	"github.com/oauth2-proxy/oauth2-proxy/v7/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const providerErrorTestCookieName = "_oauth2_proxy_provider_error"

type providerErrorResult struct {
	rw              *httptest.ResponseRecorder
	nextCalled      bool
	validatorCalled bool
	hookCalled      bool
	hookErr         error
}

type providerErrorCase struct {
	tokenHandler http.HandlerFunc
	closeServer  bool
	withHook     bool
	cancelReq    bool
	deadlineReq  bool
}

func newProviderErrorTestProvider(t *testing.T, tc providerErrorCase) *providers.OIDCProvider {
	t.Helper()
	server := httptest.NewServer(tc.tokenHandler)
	if tc.closeServer {
		server.Close()
	} else {
		t.Cleanup(server.Close)
	}
	redeemURL, err := url.Parse(server.URL + "/token")
	require.NoError(t, err)

	verifier := oidc.NewVerifier("https://issuer.example.com", &oidc.StaticKeySet{}, &oidc.Config{ClientID: "client"})
	return providers.NewOIDCProvider(&providers.ProviderData{
		ProviderName: "oidc",
		ClientID:     "client",
		ClientSecret: "secret",
		RedeemURL:    redeemURL,
		Verifier:     internaloidc.NewVerifier(verifier, internaloidc.IDTokenVerificationOptions{ClientID: "client"}),
	}, options.OIDCOptions{})
}

func newProviderErrorTestRequest(t *testing.T, store sessionsapi.SessionStore) *http.Request {
	t.Helper()
	created := time.Now().Add(-2 * time.Hour)
	expired := time.Now().Add(-time.Hour)
	session := &sessionsapi.SessionState{
		Email:        "user@example.com",
		User:         "user",
		AccessToken:  "access-token",
		IDToken:      "id-token",
		RefreshToken: "refresh-token",
		CreatedAt:    &created,
		ExpiresOn:    &expired,
	}

	saveRec := httptest.NewRecorder()
	require.NoError(t, store.Save(saveRec, httptest.NewRequest(http.MethodGet, "/", nil), session))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range saveRec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func serveWithProviderError(t *testing.T, tc providerErrorCase) *providerErrorResult {
	t.Helper()
	store, err := cookie.NewCookieSessionStore(&options.SessionOptions{}, &options.Cookie{
		Name:    providerErrorTestCookieName,
		Secret:  "0123456789abcdef0123456789abcdef",
		Path:    "/",
		Expire:  24 * time.Hour,
		Refresh: time.Hour,
	})
	require.NoError(t, err)

	res := &providerErrorResult{rw: httptest.NewRecorder()}
	opts := &StoredSessionLoaderOptions{
		SessionStore:   store,
		RefreshPeriod:  time.Hour,
		RefreshSession: newProviderErrorTestProvider(t, tc).RefreshSession,
		ValidateSession: func(context.Context, *sessionsapi.SessionState) bool {
			res.validatorCalled = true
			return true
		},
	}
	if tc.withHook {
		opts.OnProviderError = func(rw http.ResponseWriter, _ *http.Request, err error) {
			res.hookCalled = true
			res.hookErr = err
			rw.WriteHeader(http.StatusTeapot)
		}
	}

	req := newProviderErrorTestRequest(t, store)
	if tc.cancelReq {
		ctx, cancel := context.WithCancel(req.Context())
		cancel()
		req = req.WithContext(ctx)
	}
	if tc.deadlineReq {
		ctx, cancel := context.WithDeadline(req.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		req = req.WithContext(ctx)
	}
	req = middlewareapi.AddRequestScope(req, &middlewareapi.RequestScope{})

	handler := NewStoredSessionLoader(opts)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		res.nextCalled = true
	}))
	handler.ServeHTTP(res.rw, req)
	return res
}

func providerErrorSetCookies(rw *httptest.ResponseRecorder) []string {
	var out []string
	for _, c := range rw.Result().Cookies() {
		if strings.HasPrefix(c.Name, providerErrorTestCookieName) {
			out = append(out, c.String())
		}
	}
	return out
}

func tokenStatus(code int, body string) http.HandlerFunc {
	return func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(code)
		_, _ = rw.Write([]byte(body))
	}
}

func TestLoadSession_ProviderUnavailable_DefaultResponse(t *testing.T) {
	res := serveWithProviderError(t, providerErrorCase{tokenHandler: tokenStatus(http.StatusServiceUnavailable, "")})

	assert.Equal(t, http.StatusServiceUnavailable, res.rw.Code)
	assert.Empty(t, providerErrorSetCookies(res.rw))
	assert.False(t, res.nextCalled)
	assert.False(t, res.validatorCalled)
}

func TestLoadSession_ProviderUnavailable_CallsHook(t *testing.T) {
	cases := map[string]providerErrorCase{
		"token endpoint 503": {tokenHandler: tokenStatus(http.StatusServiceUnavailable, "")},
		"token endpoint 500": {tokenHandler: tokenStatus(http.StatusInternalServerError, `{"error":"server_error"}`)},
		"connection refused": {tokenHandler: tokenStatus(http.StatusOK, ""), closeServer: true},
		"token endpoint 429": {tokenHandler: tokenStatus(http.StatusTooManyRequests, "")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.withHook = true
			res := serveWithProviderError(t, tc)

			assert.True(t, res.hookCalled)
			assert.ErrorIs(t, res.hookErr, sessionsapi.ErrProviderUnavailable)
			assert.Equal(t, http.StatusTeapot, res.rw.Code)
			assert.Empty(t, providerErrorSetCookies(res.rw))
			assert.False(t, res.nextCalled)
			assert.False(t, res.validatorCalled)
		})
	}
}

func TestLoadSession_ProviderUnavailable_ClientGone(t *testing.T) {
	res := serveWithProviderError(t, providerErrorCase{
		tokenHandler: tokenStatus(http.StatusServiceUnavailable, ""),
		withHook:     true,
		cancelReq:    true,
	})

	assert.False(t, res.hookCalled)
	assert.False(t, res.nextCalled)
	assert.Empty(t, providerErrorSetCookies(res.rw))
	assert.Empty(t, res.rw.Header())
	assert.Empty(t, res.rw.Body.String())
}

func TestLoadSession_ProviderFatalError_ClearsCookie(t *testing.T) {
	cases := map[string]string{
		"invalid_grant":  `{"error":"invalid_grant"}`,
		"invalid_client": `{"error":"invalid_client"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			res := serveWithProviderError(t, providerErrorCase{
				tokenHandler: tokenStatus(http.StatusBadRequest, body),
				withHook:     true,
			})

			assert.False(t, res.hookCalled)
			assert.True(t, res.nextCalled)
			assert.NotEmpty(t, providerErrorSetCookies(res.rw))
			for _, c := range res.rw.Result().Cookies() {
				assert.Empty(t, c.Value)
			}
		})
	}
}

func TestLoadSession_ProviderPermanentError_ClearsCookie(t *testing.T) {
	cases := map[string]struct {
		code int
		body string
	}{
		"400 invalid_request":       {http.StatusBadRequest, `{"error":"invalid_request"}`},
		"401 unauthorized_client":   {http.StatusUnauthorized, `{"error":"unauthorized_client"}`},
		"200 with an error in body": {http.StatusOK, `{"error":"access_denied"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res := serveWithProviderError(t, providerErrorCase{
				tokenHandler: tokenStatus(tc.code, tc.body),
				withHook:     true,
			})

			assert.False(t, res.hookCalled)
			assert.True(t, res.nextCalled)
			assert.NotEmpty(t, providerErrorSetCookies(res.rw))
		})
	}
}

func TestLoadSession_ProviderServerDeadline_CallsHook(t *testing.T) {
	res := serveWithProviderError(t, providerErrorCase{
		tokenHandler: tokenStatus(http.StatusServiceUnavailable, ""),
		withHook:     true,
		deadlineReq:  true,
	})

	assert.True(t, res.hookCalled)
	assert.Equal(t, http.StatusTeapot, res.rw.Code)
	assert.Empty(t, providerErrorSetCookies(res.rw))
}

func TestLoadSession_ProviderInvalidIDToken_ClearsCookie(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"access_token":  "new-access-token",
		"refresh_token": "new-refresh-token",
		"token_type":    "Bearer",
		"expires_in":    3600,
		"id_token":      "not-a-jwt",
	})
	require.NoError(t, err)

	res := serveWithProviderError(t, providerErrorCase{
		tokenHandler: tokenStatus(http.StatusOK, string(body)),
		withHook:     true,
	})

	assert.False(t, res.hookCalled)
	assert.True(t, res.nextCalled)
	assert.NotEmpty(t, providerErrorSetCookies(res.rw))
}

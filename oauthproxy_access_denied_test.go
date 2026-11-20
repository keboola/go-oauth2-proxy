package oauthproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	middlewareapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/middleware"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/cookies"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const accessDeniedEmail = "denied@example.com"

type groupsProvider struct {
	*TestProvider
	groups []string
}

func (p *groupsProvider) EnrichSession(_ context.Context, s *sessions.SessionState) error {
	s.Groups = p.groups
	return nil
}

func deniedCallback(t *testing.T, opts *options.Options) *httptest.ResponseRecorder {
	t.Helper()
	opts.Cookie.Secure = false
	opts.Providers[0].AllowedGroups = []string{"allowed"}
	require.NoError(t, validation.Validate(opts))

	proxy, err := NewOAuthProxy(opts, func(string) bool { return true })
	require.NoError(t, err)
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token": "my_auth_token"}`))
	}))
	t.Cleanup(providerServer.Close)
	providerURL, err := url.Parse(providerServer.URL)
	require.NoError(t, err)
	provider := NewTestProvider(providerURL, accessDeniedEmail)
	provider.ValidToken = true
	provider.ProviderData.AllowedGroups = map[string]struct{}{"allowed": {}}
	proxy.provider = &groupsProvider{TestProvider: provider, groups: []string{"other"}}

	csrf, err := cookies.NewCSRF(proxy.CookieOptions, "")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/oauth2/callback?code=callback_code&state="+encodeState(csrf.HashOAuthState(), "%2F", false), nil)
	req = middlewareapi.AddRequestScope(req, &middlewareapi.RequestScope{})
	csrfCookie, err := csrf.SetCookie(httptest.NewRecorder(), req)
	require.NoError(t, err)
	req.AddCookie(csrfCookie)

	rw := httptest.NewRecorder()
	proxy.OAuthCallback(rw, req)
	return rw
}

func assertNoSessionCookie(t *testing.T, rw *httptest.ResponseRecorder, cookieName string) {
	t.Helper()
	for _, c := range rw.Result().Cookies() {
		if c.Name == cookieName {
			assert.Negative(t, c.MaxAge, "session cookie %q must only be cleared", c.Name)
		}
	}
}

func TestCallback_AccessDenied_UsesHook(t *testing.T) {
	var hookSession *sessions.SessionState
	opts := baseTestOptions()
	opts.OnAccessDenied = func(rw http.ResponseWriter, _ *http.Request, session *sessions.SessionState) {
		hookSession = session
		rw.WriteHeader(http.StatusTeapot)
	}

	rw := deniedCallback(t, opts)

	assert.Equal(t, http.StatusTeapot, rw.Code)
	require.NotNil(t, hookSession)
	assert.Equal(t, accessDeniedEmail, hookSession.Email)
	assert.Equal(t, []string{"other"}, hookSession.Groups)
	assertNoSessionCookie(t, rw, opts.Cookie.Name)
}

func TestCallback_AccessDenied_NoHook(t *testing.T) {
	opts := baseTestOptions()

	rw := deniedCallback(t, opts)

	assert.Equal(t, http.StatusForbidden, rw.Code)
	assertNoSessionCookie(t, rw, opts.Cookie.Name)
}

func deniedSessionTest(t *testing.T, modifier OptionsModifier) *ProcessCookieTest {
	t.Helper()
	test, err := NewProcessCookieTestWithOptionsModifiers(func(opts *options.Options) {
		opts.Providers[0].AllowedGroups = []string{"allowed"}
		modifier(opts)
	})
	require.NoError(t, err)

	test.req = httptest.NewRequest(http.MethodGet, "/", nil)
	created := time.Now()
	require.NoError(t, test.SaveSession(&sessions.SessionState{
		Email:       accessDeniedEmail,
		Groups:      []string{"other"},
		AccessToken: "oauth_token",
		CreatedAt:   &created,
	}))
	test.rw = httptest.NewRecorder()
	return test
}

func TestProxy_AccessDenied_UsesHook(t *testing.T) {
	var hookSession *sessions.SessionState
	test := deniedSessionTest(t, func(opts *options.Options) {
		opts.OnAccessDenied = func(rw http.ResponseWriter, _ *http.Request, session *sessions.SessionState) {
			hookSession = session
			rw.WriteHeader(http.StatusTeapot)
		}
	})

	test.proxy.ServeHTTP(test.rw, test.req)

	assert.Equal(t, http.StatusTeapot, test.rw.Code)
	require.NotNil(t, hookSession)
	assert.Equal(t, accessDeniedEmail, hookSession.Email)
	assert.Equal(t, []string{"other"}, hookSession.Groups)
	assertNoSessionCookie(t, test.rw, test.opts.Cookie.Name)
}

func TestProxy_AccessDenied_ForceJSONErrors_SkipsHook(t *testing.T) {
	test := deniedSessionTest(t, func(opts *options.Options) {
		opts.ForceJSONErrors = true
		opts.OnAccessDenied = func(http.ResponseWriter, *http.Request, *sessions.SessionState) {
			t.Error("hook must not run for JSON errors")
		}
	})

	test.proxy.ServeHTTP(test.rw, test.req)

	assert.Equal(t, http.StatusForbidden, test.rw.Code)
	assert.Equal(t, applicationJSON, test.rw.Header().Get("Content-Type"))
}

package oauthproxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	middlewareapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/middleware"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/sessions/cookie"
	"github.com/oauth2-proxy/oauth2-proxy/v7/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionChain_ProviderUnavailable_UsesHook(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(tokenServer.Close)
	redeemURL, err := url.Parse(tokenServer.URL + "/token")
	require.NoError(t, err)
	provider := providers.NewOIDCProvider(&providers.ProviderData{
		ProviderName: "oidc",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedeemURL:    redeemURL,
	}, options.OIDCOptions{})

	var hookErr error
	opts := baseTestOptions()
	opts.Cookie.Refresh = time.Hour
	opts.OnProviderError = func(rw http.ResponseWriter, _ *http.Request, err error) {
		hookErr = err
		rw.WriteHeader(http.StatusTeapot)
	}

	store, err := cookie.NewCookieSessionStore(&opts.Session, &opts.Cookie)
	require.NoError(t, err)
	created := time.Now().Add(-2 * time.Hour)
	expired := time.Now().Add(-time.Hour)
	saveRec := httptest.NewRecorder()
	require.NoError(t, store.Save(saveRec, httptest.NewRequest(http.MethodGet, "/", nil), &sessions.SessionState{
		Email:        "user@example.com",
		RefreshToken: "refresh-token",
		CreatedAt:    &created,
		ExpiresOn:    &expired,
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range saveRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req = middlewareapi.AddRequestScope(req, &middlewareapi.RequestScope{})

	rw := httptest.NewRecorder()
	buildSessionChain(opts, provider, store, nil).Then(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("next handler must not run")
	})).ServeHTTP(rw, req)

	assert.Equal(t, http.StatusTeapot, rw.Code)
	assert.ErrorIs(t, hookErr, sessions.ErrProviderUnavailable)
	assert.Empty(t, rw.Result().Cookies())
}

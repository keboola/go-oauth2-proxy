package providers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func refreshWithTokenHandler(t *testing.T, handler http.HandlerFunc, closeServer bool) error {
	t.Helper()
	server := httptest.NewServer(handler)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	if closeServer {
		server.Close()
	} else {
		defer server.Close()
	}

	_, err = newOIDCProvider(serverURL, true).RefreshSession(t.Context(), &sessions.SessionState{RefreshToken: refreshToken})
	require.Error(t, err)
	return err
}

func TestOIDCProviderRefreshSession_TokenEndpointStatusKeepsRetrieveError(t *testing.T) {
	err := refreshWithTokenHandler(t, func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusServiceUnavailable)
	}, false)

	var retrieveErr *oauth2.RetrieveError
	require.ErrorAs(t, err, &retrieveErr)
	assert.Equal(t, http.StatusServiceUnavailable, retrieveErr.Response.StatusCode)
}

func TestOIDCProviderRefreshSession_TokenEndpointDownKeepsURLError(t *testing.T) {
	err := refreshWithTokenHandler(t, func(http.ResponseWriter, *http.Request) {}, true)

	var urlErr *url.Error
	assert.ErrorAs(t, err, &urlErr)
}

func TestOIDCProviderRefreshSession_InvalidIDTokenHidesTokenErrors(t *testing.T) {
	body, err := json.Marshal(redeemTokenResponse{
		AccessToken:  accessToken,
		ExpiresIn:    10,
		TokenType:    "Bearer",
		RefreshToken: refreshToken,
		IDToken:      "not-a-jwt",
	})
	require.NoError(t, err)

	err = refreshWithTokenHandler(t, func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Add("content-type", "application/json")
		_, _ = rw.Write(body)
	}, false)

	var retrieveErr *oauth2.RetrieveError
	var urlErr *url.Error
	assert.False(t, errors.As(err, &retrieveErr))
	assert.False(t, errors.As(err, &urlErr))
}

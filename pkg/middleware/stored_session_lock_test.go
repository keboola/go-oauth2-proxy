package middleware

import (
	"net/http"
	"testing"
	"time"

	sessionsapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefresh_LockWaitTimeout_KeepsCookie(t *testing.T) {
	r := newRedisSessionTest(t, refreshDueSession(nil))
	sessionKey := r.mr.Keys()[0]
	require.NoError(t, r.mr.Set(sessionKey+".lock", "held-by-another-request"))
	opts := r.loaderOptions(t)
	opts.RefreshLockDuration = 50 * time.Millisecond
	opts.RefreshObtainTimeout = 50 * time.Millisecond

	res := serveStoredSession(t, opts, r.req, true)

	assert.Equal(t, http.StatusServiceUnavailable, res.rw.Code)
	assert.False(t, expiresSessionCookie(res.rw), "the session cookie must not be cleared")
	assert.False(t, res.nextCalled)
	require.True(t, res.hookCalled)
	assert.ErrorIs(t, res.hookErr, sessionsapi.ErrStoreUnavailable)
}

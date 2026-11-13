package middleware

import (
	"context"
	"net/http"
	"sync/atomic"
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

func TestRefresh_SlowRefresh_RefreshesOnce(t *testing.T) {
	r := newRedisSessionTest(t, refreshDueSession(nil))
	waiting := make(chan struct{}, 1)
	r.store.afterNotObtained = func() {
		select {
		case waiting <- struct{}{}:
		default:
		}
	}
	reqB := r.req.Clone(context.Background())

	var refreshes atomic.Int32
	var resB *storeErrorResult
	done := make(chan struct{})
	opts := r.loaderOptions(t)
	opts.RefreshSession = func(_ context.Context, s *sessionsapi.SessionState) (bool, error) {
		if refreshes.Add(1) == 1 {
			r.mr.FastForward(20 * time.Second)
			optsB := *opts
			go func() {
				defer close(done)
				resB = serveStoredSession(t, &optsB, reqB, true)
			}()
			select {
			case <-waiting:
			case <-time.After(2 * time.Second):
			}
		}
		s.RefreshToken = "rotated-refresh-token"
		return true, nil
	}

	resA := serveStoredSession(t, opts, r.req, true)
	<-done

	assert.Equal(t, int32(1), refreshes.Load())
	assert.True(t, resA.nextCalled)
	assert.True(t, resB.nextCalled)
	assert.False(t, resB.hookCalled)
}

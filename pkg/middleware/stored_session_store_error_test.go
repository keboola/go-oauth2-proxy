package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	middlewareapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/middleware"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	sessionsapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/sessions/persistence"
	redisstore "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/sessions/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const storeErrorTestCookieName = "_oauth2_proxy"

type storeErrorResult struct {
	rw          *httptest.ResponseRecorder
	nextCalled  bool
	hookCalled  bool
	hookErr     error
	hookRequest *http.Request
}

func serveStoredSession(t *testing.T, opts *StoredSessionLoaderOptions, req *http.Request, withHook bool) *storeErrorResult {
	t.Helper()
	res := &storeErrorResult{rw: httptest.NewRecorder()}
	if withHook {
		opts.OnSessionStoreError = func(rw http.ResponseWriter, r *http.Request, err error) {
			res.hookCalled = true
			res.hookErr = err
			res.hookRequest = r
			rw.WriteHeader(http.StatusServiceUnavailable)
		}
	}
	req = middlewareapi.AddRequestScope(req, &middlewareapi.RequestScope{})
	handler := NewStoredSessionLoader(opts)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		res.nextCalled = true
	}))
	handler.ServeHTTP(res.rw, req)
	return res
}

func expiresSessionCookie(rw *httptest.ResponseRecorder) bool {
	for _, c := range rw.Result().Cookies() {
		if c.Name != storeErrorTestCookieName {
			continue
		}
		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(time.Now())) {
			return true
		}
	}
	return false
}

func refreshDueSession(lock sessionsapi.Lock) *sessionsapi.SessionState {
	created := time.Now().Add(-2 * time.Hour)
	expires := time.Now().Add(time.Hour)
	return &sessionsapi.SessionState{
		RefreshToken: "refresh-token",
		CreatedAt:    &created,
		ExpiresOn:    &expires,
		Lock:         lock,
	}
}

func unavailable(msg string) error {
	return fmt.Errorf("%w: %s", sessionsapi.ErrStoreUnavailable, msg)
}

func TestLoadSession_StoreUnavailable_KeepsCookie(t *testing.T) {
	store := &fakeSessionStore{
		LoadFunc: func(*http.Request) (*sessionsapi.SessionState, error) {
			return nil, unavailable("dial")
		},
	}
	var cleared bool
	store.ClearFunc = func(http.ResponseWriter, *http.Request) error {
		cleared = true
		return nil
	}

	res := serveStoredSession(t, &StoredSessionLoaderOptions{SessionStore: store}, httptest.NewRequest(http.MethodGet, "/", nil), false)

	assert.Equal(t, http.StatusServiceUnavailable, res.rw.Code)
	assert.False(t, cleared)
	assert.False(t, expiresSessionCookie(res.rw))
	assert.False(t, res.nextCalled)
}

func TestLoadSession_SessionNotFound_ClearsCookie(t *testing.T) {
	var cleared bool
	store := &fakeSessionStore{
		LoadFunc: func(*http.Request) (*sessionsapi.SessionState, error) {
			return nil, errors.New("failed to load the session state with the ticket: session does not exist")
		},
		ClearFunc: func(http.ResponseWriter, *http.Request) error {
			cleared = true
			return nil
		},
	}

	res := serveStoredSession(t, &StoredSessionLoaderOptions{SessionStore: store}, httptest.NewRequest(http.MethodGet, "/", nil), true)

	assert.True(t, cleared)
	assert.False(t, res.hookCalled)
	assert.True(t, res.nextCalled)
	assert.Equal(t, http.StatusOK, res.rw.Code)
}

func TestRefresh_LockUnavailable_KeepsCookie(t *testing.T) {
	var cleared bool
	store := &fakeSessionStore{
		LoadFunc: func(*http.Request) (*sessionsapi.SessionState, error) {
			return refreshDueSession(&testLock{obtainError: unavailable("dial")}), nil
		},
		ClearFunc: func(http.ResponseWriter, *http.Request) error {
			cleared = true
			return nil
		},
	}
	opts := &StoredSessionLoaderOptions{
		SessionStore:  store,
		RefreshPeriod: time.Hour,
		RefreshSession: func(context.Context, *sessionsapi.SessionState) (bool, error) {
			t.Fatal("refresh must not run without the lock")
			return false, nil
		},
		ValidateSession: func(context.Context, *sessionsapi.SessionState) bool { return true },
	}

	res := serveStoredSession(t, opts, httptest.NewRequest(http.MethodGet, "/", nil), false)

	assert.Equal(t, http.StatusServiceUnavailable, res.rw.Code)
	assert.False(t, cleared)
	assert.False(t, res.nextCalled)
}

func TestOnSessionStoreError_HookCalled(t *testing.T) {
	store := &fakeSessionStore{
		LoadFunc: func(*http.Request) (*sessionsapi.SessionState, error) {
			return nil, unavailable("dial")
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/some/path", nil)

	res := serveStoredSession(t, &StoredSessionLoaderOptions{SessionStore: store}, req, true)

	require.True(t, res.hookCalled)
	assert.ErrorIs(t, res.hookErr, sessionsapi.ErrStoreUnavailable)
	assert.Equal(t, "/some/path", res.hookRequest.URL.Path)
	assert.False(t, res.nextCalled)
}

type realStore struct {
	persistence.Store
	afterFirstLoad func()
	afterObtain    func()
}

func (s *realStore) Load(ctx context.Context, key string) ([]byte, error) {
	value, err := s.Store.Load(ctx, key)
	if s.afterFirstLoad != nil {
		s.afterFirstLoad()
		s.afterFirstLoad = nil
	}
	return value, err
}

func (s *realStore) Lock(key string) sessionsapi.Lock {
	return &realLock{Lock: s.Store.Lock(key), afterObtain: s.afterObtain}
}

type realLock struct {
	sessionsapi.Lock
	afterObtain func()
}

func (l *realLock) Obtain(ctx context.Context, expiration time.Duration) error {
	err := l.Lock.Obtain(ctx, expiration)
	if err == nil && l.afterObtain != nil {
		l.afterObtain()
	}
	return err
}

type redisSessionTest struct {
	mr      *miniredis.Miniredis
	store   *realStore
	manager *persistence.Manager
	req     *http.Request
}

func newRedisSessionTest(t *testing.T, session *sessionsapi.SessionState) *redisSessionTest {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	cookieOpts := &options.Cookie{
		Name:    storeErrorTestCookieName,
		Secret:  "0123456789abcdefghijklmnopqrstuv",
		Expire:  24 * time.Hour,
		Refresh: time.Hour,
		Path:    "/",
	}
	ss, err := redisstore.NewRedisSessionStore(&options.SessionOptions{
		Type:  options.RedisSessionStoreType,
		Redis: options.RedisStoreOptions{ConnectionURL: "redis://" + mr.Addr()},
	}, cookieOpts)
	require.NoError(t, err)

	store := &realStore{Store: ss.(*persistence.Manager).Store}
	manager := persistence.NewManager(store, cookieOpts)

	saveRW := httptest.NewRecorder()
	require.NoError(t, manager.Save(saveRW, httptest.NewRequest(http.MethodGet, "/", nil), session))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range saveRW.Result().Cookies() {
		req.AddCookie(c)
	}
	require.Len(t, mr.Keys(), 1)

	return &redisSessionTest{mr: mr, store: store, manager: manager, req: req}
}

func (r *redisSessionTest) loaderOptions(t *testing.T) *StoredSessionLoaderOptions {
	return &StoredSessionLoaderOptions{
		SessionStore:  r.manager,
		RefreshPeriod: time.Hour,
		RefreshSession: func(context.Context, *sessionsapi.SessionState) (bool, error) {
			t.Fatal("refresh must not run while the store is down")
			return false, nil
		},
		ValidateSession: func(context.Context, *sessionsapi.SessionState) bool { return true },
	}
}

func TestManagerLoad_RedisDown_KeepsCookie(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(r *redisSessionTest)
	}{
		{
			name:  "load",
			setup: func(r *redisSessionTest) { r.mr.Close() },
		},
		{
			name:  "lock",
			setup: func(r *redisSessionTest) { r.store.afterFirstLoad = r.mr.Close },
		},
		{
			name:  "reload under lock",
			setup: func(r *redisSessionTest) { r.store.afterObtain = r.mr.Close },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRedisSessionTest(t, refreshDueSession(nil))
			tc.setup(r)

			res := serveStoredSession(t, r.loaderOptions(t), r.req, true)

			assert.Equal(t, http.StatusServiceUnavailable, res.rw.Code)
			assert.False(t, expiresSessionCookie(res.rw), "the session cookie must not be cleared")
			assert.False(t, res.nextCalled)
			require.True(t, res.hookCalled)
			assert.ErrorIs(t, res.hookErr, sessionsapi.ErrStoreUnavailable)
		})
	}
}

func TestManagerLoad_SessionDeleted_ClearsCookie(t *testing.T) {
	r := newRedisSessionTest(t, refreshDueSession(nil))
	for _, key := range r.mr.Keys() {
		r.mr.Del(key)
	}

	res := serveStoredSession(t, r.loaderOptions(t), r.req, true)

	assert.True(t, expiresSessionCookie(res.rw), "a missing session must clear the cookie")
	assert.False(t, res.hookCalled)
	assert.True(t, res.nextCalled)
}

func TestManagerLoad_RedisDownAfterRefresh_KeepsCookie(t *testing.T) {
	r := newRedisSessionTest(t, refreshDueSession(nil))
	opts := r.loaderOptions(t)
	var refreshed bool
	opts.RefreshSession = func(_ context.Context, s *sessionsapi.SessionState) (bool, error) {
		refreshed = true
		s.RefreshToken = "rotated-refresh-token"
		r.mr.Close()
		return true, nil
	}

	res := serveStoredSession(t, opts, r.req, true)

	require.True(t, refreshed)
	assert.Equal(t, http.StatusServiceUnavailable, res.rw.Code)
	assert.False(t, expiresSessionCookie(res.rw), "the session cookie must not be cleared")
	assert.False(t, res.nextCalled, "the request must not be served with tokens that were not saved")
	require.True(t, res.hookCalled)
	assert.ErrorIs(t, res.hookErr, sessionsapi.ErrStoreUnavailable)
}

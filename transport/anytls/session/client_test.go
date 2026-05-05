package session

import (
	"context"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/transport/anytls/padding"
	"github.com/metacubex/mihomo/transport/anytls/skiplist"
	"github.com/stretchr/testify/assert"
)

type mockConn struct {
	net.Conn
	closed atomic.Bool
}

func (m *mockConn) Close() error {
	m.closed.Store(true)
	return nil
}
func (m *mockConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Write(b []byte) (n int, err error) { return len(b), nil }
func (m *mockConn) SetDeadline(t time.Time) error     { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func newMockConn() *mockConn {
	return &mockConn{}
}

func makeTestPadding() *atomic.Pointer[padding.PaddingFactory] {
	p := atomic.Pointer[padding.PaddingFactory]{}
	p.Store(padding.NewPaddingFactory(padding.DefaultPaddingScheme))
	return &p
}

func newTestClient(
	idleSessionTimeout time.Duration,
	minIdleSession int,
	maxConnectionLifetime, connectionLifetimeJitter time.Duration,
	minIdleSessionForAge int,
	ensureIdleSession, ensureIdleSessionCreateRate int,
) (*Client, *sync.WaitGroup) {
	ctx := context.Background()
	pad := makeTestPadding()
	wg := &sync.WaitGroup{}

	dialOut := func(ctx context.Context) (net.Conn, error) {
		wg.Add(1)
		return newMockConn(), nil
	}

	c := NewClient(
		ctx,
		dialOut,
		pad,
		time.Second*30,
		idleSessionTimeout,
		minIdleSession,
		0,
		maxConnectionLifetime,
		connectionLifetimeJitter,
		minIdleSessionForAge,
		ensureIdleSession,
		ensureIdleSessionCreateRate,
	)
	return c, wg
}

func newTestSession(seq uint64, idleSince time.Time) *Session {
	return &Session{
		conn:      newMockConn(),
		die:       make(chan struct{}),
		streams:   make(map[uint32]*Stream),
		idleSince: idleSince,
		seq:       seq,
	}
}

func newTestSessionWithAge(seq uint64, createdAt time.Time, maxLifetime time.Duration) *Session {
	return &Session{
		conn:        newMockConn(),
		die:         make(chan struct{}),
		streams:     make(map[uint32]*Stream),
		createdAt:   createdAt,
		maxLifetime: maxLifetime,
		seq:         seq,
	}
}

func TestIdleCleanup(t *testing.T) {
	t.Run("closes idle sessions beyond timeout", func(t *testing.T) {
		c, wg := newTestClient(time.Second*5, 0, 0, 0, 0, 0, 0)
		defer c.Close()

		session := newTestSession(1, time.Now().Add(-time.Second*10))
		session.dieHook = func() {}

		c.idleSessionLock.Lock()
		c.idleSession.Insert(math.MaxUint64-1, session)
		c.idleSessionLock.Unlock()

		c.sessionsLock.Lock()
		c.sessions[1] = session
		c.sessionsLock.Unlock()

		initialLen := c.idleSession.Len()
		assert.Greater(t, initialLen, 0)

		c.idleCleanupExpTime(time.Now().Add(-time.Second * 5))

		assert.Equal(t, 0, c.idleSession.Len())
		c.sessionsLock.Lock()
		assert.Equal(t, 0, len(c.sessions))
		c.sessionsLock.Unlock()
		wg.Wait()
	})

	t.Run("preserves minIdleSession oldest sessions", func(t *testing.T) {
		c, wg := newTestClient(time.Second*5, 2, 0, 0, 0, 0, 0)
		defer c.Close()

		now := time.Now()
		for i := uint64(1); i <= 3; i++ {
			session := newTestSession(i, now.Add(-time.Second*10))
			session.dieHook = func() {}
			c.idleSessionLock.Lock()
			c.idleSession.Insert(math.MaxUint64-i, session)
			c.idleSessionLock.Unlock()
			c.sessionsLock.Lock()
			c.sessions[i] = session
			c.sessionsLock.Unlock()
		}

		c.idleCleanupExpTime(now.Add(-time.Second * 5))

		assert.Equal(t, 2, c.idleSession.Len())
		wg.Wait()
	})

	t.Run("removes closed sessions from c.sessions", func(t *testing.T) {
		c, wg := newTestClient(time.Second*5, 0, 0, 0, 0, 0, 0)
		defer c.Close()

		session := newTestSession(1, time.Now().Add(-time.Second*10))
		session.dieHook = func() {}

		c.idleSessionLock.Lock()
		c.idleSession.Insert(math.MaxUint64-1, session)
		c.idleSessionLock.Unlock()

		c.sessionsLock.Lock()
		c.sessions[1] = session
		c.sessionsLock.Unlock()

		c.idleCleanupExpTime(time.Now().Add(-time.Second * 5))

		c.sessionsLock.Lock()
		_, exists := c.sessions[1]
		c.sessionsLock.Unlock()
		assert.False(t, exists, "session should be removed from c.sessions after close")
		wg.Wait()
	})
}

func TestAgeCleanup(t *testing.T) {
	t.Run("closes expired idle sessions", func(t *testing.T) {
		c, wg := newTestClient(time.Second*30, 0, time.Second*10, 0, 0, 0, 0)
		defer c.Close()

		oldTime := time.Now().Add(-time.Second * 15)
		for i := uint64(1); i <= 3; i++ {
			session := newTestSessionWithAge(i, oldTime, time.Second*10)
			session.idleSince = oldTime
			session.dieHook = func() {}
			c.idleSessionLock.Lock()
			c.idleSession.Insert(math.MaxUint64-i, session)
			c.idleSessionLock.Unlock()
			c.sessionsLock.Lock()
			c.sessions[i] = session
			c.sessionsLock.Unlock()
		}

		c.ageCleanupExpTime(time.Now())

		c.sessionsLock.Lock()
		remaining := len(c.sessions)
		c.sessionsLock.Unlock()
		assert.Equal(t, 0, remaining)
		wg.Wait()
	})

	t.Run("does not close active sessions not in idle pool", func(t *testing.T) {
		c, wg := newTestClient(time.Second*30, 0, time.Second*10, 0, 0, 0, 0)
		defer c.Close()

		oldTime := time.Now().Add(-time.Second * 15)
		for i := uint64(1); i <= 3; i++ {
			session := newTestSessionWithAge(i, oldTime, time.Second*10)
			session.dieHook = func() {}
			c.sessionsLock.Lock()
			c.sessions[i] = session
			c.sessionsLock.Unlock()
		}

		c.ageCleanupExpTime(time.Now())

		c.sessionsLock.Lock()
		remaining := len(c.sessions)
		c.sessionsLock.Unlock()
		assert.Equal(t, 3, remaining, "active sessions not in idle pool should not be closed")
		wg.Wait()
	})

	t.Run("preserves minIdleSessionForAge floor", func(t *testing.T) {
		c, wg := newTestClient(time.Second*30, 0, time.Second*10, 0, 2, 0, 0)
		defer c.Close()

		oldTime := time.Now().Add(-time.Second * 15)
		for i := uint64(1); i <= 5; i++ {
			session := newTestSessionWithAge(i, oldTime, time.Second*10)
			session.idleSince = oldTime
			session.dieHook = func() {}
			c.idleSessionLock.Lock()
			c.idleSession.Insert(math.MaxUint64-i, session)
			c.idleSessionLock.Unlock()
			c.sessionsLock.Lock()
			c.sessions[i] = session
			c.sessionsLock.Unlock()
		}

		c.ageCleanupExpTime(time.Now())

		c.sessionsLock.Lock()
		remaining := len(c.sessions)
		c.sessionsLock.Unlock()
		assert.Equal(t, 2, remaining)
		wg.Wait()
	})

	t.Run("does not close sessions within maxLifetime", func(t *testing.T) {
		c, wg := newTestClient(time.Second*30, 0, time.Second*30, 0, 0, 0, 0)
		defer c.Close()

		recentTime := time.Now().Add(-time.Second * 5)
		for i := uint64(1); i <= 3; i++ {
			session := newTestSessionWithAge(i, recentTime, time.Second*30)
			session.idleSince = recentTime
			session.dieHook = func() {}
			c.idleSessionLock.Lock()
			c.idleSession.Insert(math.MaxUint64-i, session)
			c.idleSessionLock.Unlock()
			c.sessionsLock.Lock()
			c.sessions[i] = session
			c.sessionsLock.Unlock()
		}

		c.ageCleanupExpTime(time.Now())

		c.sessionsLock.Lock()
		remaining := len(c.sessions)
		c.sessionsLock.Unlock()
		assert.Equal(t, 3, remaining)
		wg.Wait()
	})

	t.Run("closes oldest expired sessions first", func(t *testing.T) {
		c, wg := newTestClient(time.Second*30, 0, time.Second*10, 0, 1, 0, 0)
		defer c.Close()

		veryOld := time.Now().Add(-time.Second * 30)
		old := time.Now().Add(-time.Second * 15)
		recent := time.Now().Add(-time.Second * 5)

		session1 := newTestSessionWithAge(1, veryOld, time.Second*10)
		session2 := newTestSessionWithAge(2, old, time.Second*10)
		session3 := newTestSessionWithAge(3, recent, time.Second*10)

		for _, s := range []*Session{session1, session2, session3} {
			s.idleSince = time.Now()
			s.dieHook = func() {}
			c.idleSessionLock.Lock()
			c.idleSession.Insert(math.MaxUint64-s.seq, s)
			c.idleSessionLock.Unlock()
			c.sessionsLock.Lock()
			c.sessions[s.seq] = s
			c.sessionsLock.Unlock()
		}

		c.ageCleanupExpTime(time.Now())

		c.sessionsLock.Lock()
		remaining := len(c.sessions)
		c.sessionsLock.Unlock()
		assert.Equal(t, 2, remaining)
		wg.Wait()
	})
}

func TestEnsureIdleSession(t *testing.T) {
	t.Run("does nothing when idle sessions at target", func(t *testing.T) {
		c, wg := newTestClient(time.Second*30, 0, 0, 0, 0, 5, 10)
		defer c.Close()

		for i := uint64(1); i <= 5; i++ {
			session := newTestSession(i, time.Now())
			session.dieHook = func() {}
			c.idleSessionLock.Lock()
			c.idleSession.Insert(math.MaxUint64-i, session)
			c.idleSessionLock.Unlock()
		}

		initialLen := c.idleSession.Len()
		c.ensureIdleSessionFill()
		assert.Equal(t, initialLen, c.idleSession.Len())
		wg.Wait()
	})

	t.Run("does not create sessions after shutdown", func(t *testing.T) {
		c, wg := newTestClient(time.Second*30, 0, 0, 0, 0, 5, 10)
		c.Close()

		c.ensureIdleSessionFill()

		c.idleSessionLock.Lock()
		assert.Equal(t, 0, c.idleSession.Len())
		c.idleSessionLock.Unlock()
		wg.Wait()
	})

	t.Run("creates sessions when below target", func(t *testing.T) {
		c, _ := newTestClient(time.Second*30, 0, 0, 0, 0, 5, 0)
		defer c.Close()

		initialLen := c.idleSession.Len()
		assert.Equal(t, 0, initialLen)

		c.ensureIdleSessionFill()

		for i := 0; i < 50 && c.idleSession.Len() < 5; i++ {
			time.Sleep(5 * time.Millisecond)
		}

		c.idleSessionLock.Lock()
		created := c.idleSession.Len()
		c.idleSessionLock.Unlock()
		assert.Equal(t, 5, created)
	})

	t.Run("respects ensureIdleSessionCreateRate", func(t *testing.T) {
		c, _ := newTestClient(time.Second*30, 0, 0, 0, 0, 10, 2)
		defer c.Close()

		initialLen := c.idleSession.Len()
		assert.Equal(t, 0, initialLen)

		c.ensureIdleSessionFill()

		for i := 0; i < 50 && c.idleSession.Len() < 2; i++ {
			time.Sleep(5 * time.Millisecond)
		}

		c.idleSessionLock.Lock()
		created := c.idleSession.Len()
		c.idleSessionLock.Unlock()
		assert.Equal(t, 2, created, "should only create ensureIdleSessionCreateRate sessions per cycle")
	})
}

func TestIdleSessionSkipList(t *testing.T) {
	sl := skiplist.NewSkipList[uint64, *Session]()

	session1 := &Session{seq: 1}
	session2 := &Session{seq: 2}
	session3 := &Session{seq: 3}

	sl.Insert(math.MaxUint64-1, session1)
	sl.Insert(math.MaxUint64-2, session2)
	sl.Insert(math.MaxUint64-3, session3)

	assert.Equal(t, 3, sl.Len())

	it := sl.Iterate()
	var seqs []uint64
	for it.IsNotEnd() {
		seqs = append(seqs, it.Value().seq)
		it.MoveToNext()
	}
	assert.Equal(t, []uint64{3, 2, 1}, seqs)

	sl.Remove(math.MaxUint64 - 2)
	assert.Equal(t, 2, sl.Len())
}
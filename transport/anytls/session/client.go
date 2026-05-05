package session

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/transport/anytls/padding"
	"github.com/metacubex/mihomo/transport/anytls/skiplist"
	"github.com/metacubex/mihomo/transport/anytls/util"
)

type Client struct {
	die       context.Context
	dieCancel context.CancelFunc

	dialOut util.DialOutFunc

	sessionCounter atomic.Uint64

	idleSession     *skiplist.SkipList[uint64, *Session]
	idleSessionLock sync.Mutex

	sessions     map[uint64]*Session
	sessionsLock sync.Mutex

	padding *atomic.Pointer[padding.PaddingFactory]

	idleSessionTimeout time.Duration
	minIdleSession     int
	heartbeat          time.Duration
	maxConnectionLifetime     time.Duration
	connectionLifetimeJitter  time.Duration
	minIdleSessionForAge     int
	ensureIdleSession        int
	ensureIdleSessionCreateRate int
}

func NewClient(ctx context.Context, dialOut util.DialOutFunc, _padding *atomic.Pointer[padding.PaddingFactory], idleSessionCheckInterval, idleSessionTimeout time.Duration, minIdleSession int, heartbeat time.Duration, maxConnectionLifetime time.Duration, connectionLifetimeJitter time.Duration, minIdleSessionForAge int, ensureIdleSession int, ensureIdleSessionCreateRate int) *Client {
	c := &Client{
		sessions:           make(map[uint64]*Session),
		dialOut:            dialOut,
		padding:            _padding,
		idleSessionTimeout: idleSessionTimeout,
		minIdleSession:     minIdleSession,
		heartbeat:          heartbeat,
		maxConnectionLifetime:     maxConnectionLifetime,
		connectionLifetimeJitter:  connectionLifetimeJitter,
		minIdleSessionForAge:     minIdleSessionForAge,
		ensureIdleSession:        ensureIdleSession,
		ensureIdleSessionCreateRate: ensureIdleSessionCreateRate,
	}
	if idleSessionCheckInterval <= time.Second*5 {
		idleSessionCheckInterval = time.Second * 30
	}
	if c.idleSessionTimeout <= time.Second*5 {
		c.idleSessionTimeout = time.Second * 30
	}
	c.die, c.dieCancel = context.WithCancel(ctx)
	c.idleSession = skiplist.NewSkipList[uint64, *Session]()
	util.StartRoutine(c.die, idleSessionCheckInterval, c.idleCleanup)
	if c.maxConnectionLifetime > 0 {
		util.StartRoutine(c.die, idleSessionCheckInterval, c.ageCleanup)
	}
	if c.ensureIdleSession > 0 {
		util.StartRoutine(c.die, idleSessionCheckInterval, c.ensureIdleSessionFill)
	}
	return c
}

func (c *Client) CreateStream(ctx context.Context) (net.Conn, error) {
	select {
	case <-c.die.Done():
		return nil, io.ErrClosedPipe
	default:
	}

	var session *Session
	var stream *Stream
	var err error

	session = c.getIdleSession()
	if session == nil {
		session, err = c.createSession(ctx)
	}
	if session == nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	stream, err = session.OpenStream()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	stream.dieHook = func() {
		if !session.IsClosed() {
			select {
			case <-c.die.Done():
				go session.Close()
			default:
				c.idleSessionLock.Lock()
				session.idleSince = time.Now()
				c.idleSession.Insert(math.MaxUint64-session.seq, session)
				c.idleSessionLock.Unlock()
			}
		}
	}

	return stream, nil
}

func (c *Client) getIdleSession() (idle *Session) {
	c.idleSessionLock.Lock()
	if !c.idleSession.IsEmpty() {
		it := c.idleSession.Iterate()
		idle = it.Value()
		c.idleSession.Remove(it.Key())
	}
	c.idleSessionLock.Unlock()
	return
}

func (c *Client) createSession(ctx context.Context) (*Session, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.die.Done():
		return nil, io.ErrClosedPipe
	default:
	}

	underlying, err := c.dialOut(ctx)
	if err != nil {
		return nil, err
	}

	session := NewClientSession(underlying, c.padding)
	session.seq = c.sessionCounter.Add(1)
	session.createdAt = time.Now()

	if c.connectionLifetimeJitter > 0 {
		jitter := time.Duration(rand.Int63n(int64(c.connectionLifetimeJitter)))
		session.maxLifetime = c.maxConnectionLifetime + jitter
	} else {
		session.maxLifetime = c.maxConnectionLifetime
	}

	session.dieHook = func() {
		c.idleSessionLock.Lock()
		c.idleSession.Remove(math.MaxUint64 - session.seq)
		c.idleSessionLock.Unlock()
	}

	c.sessionsLock.Lock()
	c.sessions[session.seq] = session
	c.sessionsLock.Unlock()

	session.Run(c.heartbeat)
	return session, nil
}

func (c *Client) Close() error {
	c.dieCancel()

	c.sessionsLock.Lock()
	sessionToClose := make([]*Session, 0, len(c.sessions))
	for _, session := range c.sessions {
		sessionToClose = append(sessionToClose, session)
	}
	c.sessions = make(map[uint64]*Session)
	c.sessionsLock.Unlock()

	for _, session := range sessionToClose {
		session.Close()
	}

	return nil
}

func (c *Client) idleCleanup() {
	c.idleCleanupExpTime(time.Now().Add(-c.idleSessionTimeout))
}

func (c *Client) idleCleanupExpTime(expTime time.Time) {
	activeCount := 0
	sessionToClose := make([]*Session, 0, c.idleSession.Len())

	c.idleSessionLock.Lock()
	it := c.idleSession.Iterate()
	for it.IsNotEnd() {
		session := it.Value()
		key := it.Key()
		it.MoveToNext()

		if !session.idleSince.Before(expTime) {
			activeCount++
			continue
		}

		if activeCount < c.minIdleSession {
			session.idleSince = time.Now()
			activeCount++
			continue
		}

		sessionToClose = append(sessionToClose, session)
		c.idleSession.Remove(key)
	}
	c.idleSessionLock.Unlock()

	for _, session := range sessionToClose {
		c.sessionsLock.Lock()
		delete(c.sessions, session.seq)
		c.sessionsLock.Unlock()
		session.Close()
	}
}

func (c *Client) ageCleanup() {
	if c.maxConnectionLifetime <= 0 {
		return
	}
	c.ageCleanupExpTime(time.Now())
}

func (c *Client) ageCleanupExpTime(now time.Time) {
	if c.maxConnectionLifetime <= 0 {
		return
	}

	sessionToClose := make([]*Session, 0)

	c.idleSessionLock.Lock()
	it := c.idleSession.Iterate()
	for it.IsNotEnd() {
		session := it.Value()
		key := it.Key()
		it.MoveToNext()

		age := now.Sub(session.createdAt)
		if age >= session.maxLifetime && session.maxLifetime > 0 {
			sessionToClose = append(sessionToClose, session)
			c.idleSession.Remove(key)
		}
	}
	c.idleSessionLock.Unlock()

	if len(sessionToClose) == 0 {
		return
	}

	sortByCreatedAt(sessionToClose)

	toClose := len(sessionToClose)
	if toClose > len(sessionToClose)-c.minIdleSessionForAge {
		toClose = len(sessionToClose) - c.minIdleSessionForAge
	}
	if toClose <= 0 {
		return
	}

	toCloseSessions := sessionToClose[:toClose]
	for _, session := range toCloseSessions {
		c.sessionsLock.Lock()
		delete(c.sessions, session.seq)
		c.sessionsLock.Unlock()
		session.Close()
	}
}

func sortByCreatedAt(sessions []*Session) {
	for i := 1; i < len(sessions); i++ {
		for j := i; j > 0 && sessions[j-1].createdAt.After(sessions[j].createdAt); j-- {
			sessions[j-1], sessions[j] = sessions[j], sessions[j-1]
		}
	}
}

func (c *Client) ensureIdleSessionFill() {
	if c.ensureIdleSession <= 0 {
		return
	}

	select {
	case <-c.die.Done():
		return
	default:
	}

	c.idleSessionLock.Lock()
	currentIdle := c.idleSession.Len()
	c.idleSessionLock.Unlock()

	deficit := c.ensureIdleSession - currentIdle
	if deficit <= 0 {
		return
	}

	toCreate := deficit
	if c.ensureIdleSessionCreateRate > 0 && toCreate > c.ensureIdleSessionCreateRate {
		toCreate = c.ensureIdleSessionCreateRate
	}

	for i := 0; i < toCreate; i++ {
		go func() {
			select {
			case <-c.die.Done():
				return
			default:
			}
			session, err := c.createSession(context.Background())
			if err != nil {
				return
			}
			c.idleSessionLock.Lock()
			session.idleSince = time.Now()
			c.idleSession.Insert(math.MaxUint64-session.seq, session)
			c.idleSessionLock.Unlock()
		}()
	}
}
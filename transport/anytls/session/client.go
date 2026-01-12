package session

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/log"
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

	// Idle timeout management
	idleSessionTimeout time.Duration
	minIdleSession     int

	// Proactive pool management (NEW)
	ensureIdleSession           int // Target pool size
	ensureIdleSessionCreateRate int // Max sessions per cleanup cycle
	minIdleSessionForAge        int // Separate protection for age-based cleanup

	// Age-based rotation (NEW)
	maxConnectionLifetime    time.Duration
	connectionLifetimeJitter time.Duration
}

// ClientConfig contains configuration for session client
type ClientConfig struct {
	IdleSessionCheckInterval    time.Duration
	IdleSessionTimeout          time.Duration
	MinIdleSession              int
	EnsureIdleSession           int           // Proactive pool size
	EnsureIdleSessionCreateRate int           // Max sessions per cycle
	MinIdleSessionForAge        int           // Age-based protection
	MaxConnectionLifetime       time.Duration // Age-based rotation
	ConnectionLifetimeJitter    time.Duration // Randomization
}

func NewClient(ctx context.Context, dialOut util.DialOutFunc, _padding *atomic.Pointer[padding.PaddingFactory], config ClientConfig) *Client {
	c := &Client{
		sessions:                    make(map[uint64]*Session),
		dialOut:                     dialOut,
		padding:                     _padding,
		idleSessionTimeout:          config.IdleSessionTimeout,
		minIdleSession:              config.MinIdleSession,
		ensureIdleSession:           config.EnsureIdleSession,
		ensureIdleSessionCreateRate: config.EnsureIdleSessionCreateRate,
		minIdleSessionForAge:        config.MinIdleSessionForAge,
		maxConnectionLifetime:       config.MaxConnectionLifetime,
		connectionLifetimeJitter:    config.ConnectionLifetimeJitter,
	}

	// Set defaults
	idleSessionCheckInterval := config.IdleSessionCheckInterval
	if idleSessionCheckInterval <= time.Second*5 {
		idleSessionCheckInterval = time.Second * 30
	}
	if c.idleSessionTimeout <= time.Second*5 {
		c.idleSessionTimeout = time.Second * 30
	}

	c.die, c.dieCancel = context.WithCancel(ctx)
	c.idleSession = skiplist.NewSkipList[uint64, *Session]()
	util.StartRoutine(c.die, idleSessionCheckInterval, c.cleanup)
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
		// If Session is not closed, put this Stream to pool
		if !session.IsClosed() {
			select {
			case <-c.die.Done():
				// Now client has been closed
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
	underlying, err := c.dialOut(ctx)
	if err != nil {
		return nil, err
	}

	session := NewClientSession(underlying, c.padding)
	session.seq = c.sessionCounter.Add(1)
	session.createdAt = time.Now() // Track creation time for age-based rotation
	session.dieHook = func() {
		c.idleSessionLock.Lock()
		c.idleSession.Remove(math.MaxUint64 - session.seq)
		c.idleSessionLock.Unlock()

		c.sessionsLock.Lock()
		delete(c.sessions, session.seq)
		c.sessionsLock.Unlock()
	}

	c.sessionsLock.Lock()
	c.sessions[session.seq] = session
	c.sessionsLock.Unlock()

	session.Run()
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

// cleanup performs unified session pool maintenance:
// 1. Idle timeout cleanup with minIdleSession protection
// 2. Age-based rotation with minIdleSessionForAge protection and jitter
// 3. Proactive session creation to maintain ensureIdleSession target
func (c *Client) cleanup() {
	now := time.Now()
	idleExpTime := now.Add(-c.idleSessionTimeout)

	idleSessionsToClose := make([]*Session, 0)
	ageSessionsToClose := make([]*Session, 0)
	idleActiveCount := 0
	ageActiveCount := 0

	c.idleSessionLock.Lock()
	it := c.idleSession.Iterate()
	for it.IsNotEnd() {
		session := it.Value()
		key := it.Key()
		it.MoveToNext()

		shouldCloseIdle := false
		shouldCloseAge := false

		// Check idle timeout
		if session.idleSince.Before(idleExpTime) {
			if idleActiveCount >= c.minIdleSession {
				shouldCloseIdle = true
			} else {
				session.idleSince = now // Reset to keep this session
				idleActiveCount++
			}
		} else {
			idleActiveCount++
		}

		// Check age-based expiration (if enabled)
		if c.maxConnectionLifetime > 0 && !shouldCloseIdle {
			// Calculate session-specific lifetime with jitter
			sessionLifetime := c.maxConnectionLifetime
			if c.connectionLifetimeJitter > 0 {
				// Use session seq as seed for deterministic jitter per session
				jitterRange := int64(c.connectionLifetimeJitter)
				jitterOffset := int64(session.seq) % (jitterRange * 2)
				sessionLifetime = sessionLifetime - c.connectionLifetimeJitter + time.Duration(jitterOffset)
			}

			ageExpTime := session.createdAt.Add(sessionLifetime)
			if now.After(ageExpTime) {
				if ageActiveCount >= c.minIdleSessionForAge {
					shouldCloseAge = true
				} else {
					ageActiveCount++
				}
			}
		}

		// Close session if either condition met
		if shouldCloseIdle || shouldCloseAge {
			if shouldCloseIdle {
				idleSessionsToClose = append(idleSessionsToClose, session)
			} else {
				ageSessionsToClose = append(ageSessionsToClose, session)
			}
			c.idleSession.Remove(key)
		}
	}

	currentPoolSize := c.idleSession.Len()
	c.idleSessionLock.Unlock()

	// Debug logging for idle cleanup
	if len(idleSessionsToClose) > 0 {
		log.Debugln("[AnyTLS] Idle cleanup: found %d idle sessions, closing %d (keeping %d protected)",
			currentPoolSize+len(idleSessionsToClose), len(idleSessionsToClose), idleActiveCount)
	}

	// Debug logging for age cleanup
	if len(ageSessionsToClose) > 0 {
		log.Debugln("[AnyTLS] Age cleanup: closing %d aged sessions (keeping %d protected)",
			len(ageSessionsToClose), ageActiveCount)
	}

	// Close sessions
	for _, session := range idleSessionsToClose {
		session.Close()
	}
	for _, session := range ageSessionsToClose {
		session.Close()
	}

	// Proactive session creation (ensureIdleSession)
	if c.ensureIdleSession > 0 {
		deficit := c.ensureIdleSession - currentPoolSize
		if deficit > 0 {
			// Apply rate limiting
			toCreate := deficit
			if c.ensureIdleSessionCreateRate > 0 && toCreate > c.ensureIdleSessionCreateRate {
				toCreate = c.ensureIdleSessionCreateRate
			}

			log.Debugln("[AnyTLS] Proactive pool maintenance: current=%d, target=%d, creating %d sessions",
				currentPoolSize, c.ensureIdleSession, toCreate)

			// Create sessions asynchronously to avoid blocking cleanup
			for i := 0; i < toCreate; i++ {
				go func() {
					// Use background context for proactive creation
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					session, err := c.createSession(ctx)
					if err != nil {
						log.Debugln("[AnyTLS] Failed to create proactive session: %v", err)
						return
					}

					// Immediately put into idle pool
					c.idleSessionLock.Lock()
					session.idleSince = time.Now()
					c.idleSession.Insert(math.MaxUint64-session.seq, session)
					c.idleSessionLock.Unlock()

					log.Debugln("[AnyTLS] Created proactive session #%d", session.seq)
				}()
			}
		}
	}
}

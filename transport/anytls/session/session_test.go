package session

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/transport/anytls/padding"
)

type scriptedConn struct {
	net.Conn
	closed       atomic.Bool
	writeChan    chan []byte
	readChan     chan []byte
	onHeartRequest func()
}

func newScriptedConn() *scriptedConn {
	return &scriptedConn{
		writeChan: make(chan []byte, 100),
		readChan:  make(chan []byte, 10),
	}
}

func (s *scriptedConn) Close() error {
	s.closed.Store(true)
	return nil
}

func (s *scriptedConn) Read(b []byte) (n int, err error) {
	data, ok := <-s.readChan
	if !ok {
		return 0, nil
	}
	copy(b, data)
	return len(data), nil
}

func (s *scriptedConn) Write(b []byte) (n int, err error) {
	s.writeChan <- b
	if s.onHeartRequest != nil {
		for i := 0; i <= len(b)-7; i++ {
			if b[i] == cmdHeartRequest {
				s.onHeartRequest()
				break
			}
		}
	}
	return len(b), nil
}

func (s *scriptedConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (s *scriptedConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
}

func (s *scriptedConn) SetDeadline(t time.Time) error   { return nil }
func (s *scriptedConn) SetReadDeadline(t time.Time) error  { return nil }
func (s *scriptedConn) SetWriteDeadline(t time.Time) error { return nil }

func (s *scriptedConn) inject(data []byte) {
	s.readChan <- data
}

func makeHeartbeatTestPadding() *atomic.Pointer[padding.PaddingFactory] {
	p := atomic.Pointer[padding.PaddingFactory]{}
	p.Store(padding.NewPaddingFactory(padding.DefaultPaddingScheme))
	return &p
}

func TestHeartbeatClosesOnMissedAck(t *testing.T) {
	conn := newScriptedConn()
	pad := makeHeartbeatTestPadding()
	session := NewClientSession(conn, pad)

	session.setTestHooks(sessionTestHooks{
		sleepFn: func(d time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		},
		afterFn: func(d time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		},
	})

	conn.inject([]byte{cmdServerSettings, 0, 0, 0, 0, 0, 0})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		session.Run(time.Millisecond * 100)
	}()
	wg.Wait()

	time.Sleep(time.Millisecond * 20)
	if !conn.closed.Load() {
		t.Fatal("session did not close when heartbeat ack was missed")
	}
}

func TestHeartbeatContinuesOnResponse(t *testing.T) {
	var heartbeatSeen atomic.Bool
	conn := newScriptedConn()
	conn.onHeartRequest = func() {
		if !heartbeatSeen.Load() {
			heartbeatSeen.Store(true)
			go func() {
				conn.inject([]byte{cmdHeartResponse, 0, 0, 0, 0, 0, 0})
			}()
		}
	}
	pad := makeHeartbeatTestPadding()
	session := NewClientSession(conn, pad)
	session.sendPadding = false

	session.setTestHooks(sessionTestHooks{
		sleepFn: func(d time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		},
		afterFn: func(d time.Duration) <-chan time.Time {
			return time.After(d)
		},
	})

	conn.inject([]byte{cmdServerSettings, 0, 0, 0, 0, 0, 0})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		session.Run(time.Millisecond * 100)
	}()
	wg.Wait()

	time.Sleep(time.Millisecond * 50)
	if !heartbeatSeen.Load() {
		t.Fatal("heartbeat request was never sent")
	}
	if conn.closed.Load() {
		t.Fatal("session closed despite receiving heartbeat response")
	}
}

func TestHeartbeatSendsCorrectFrame(t *testing.T) {
	conn := newScriptedConn()
	pad := makeHeartbeatTestPadding()
	session := NewClientSession(conn, pad)
	session.sendPadding = false

	session.setTestHooks(sessionTestHooks{
		sleepFn: func(d time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		},
		afterFn: func(d time.Duration) <-chan time.Time {
			return time.After(d)
		},
	})

	conn.inject([]byte{cmdServerSettings, 0, 0, 0, 0, 0, 0})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		session.Run(time.Millisecond * 100)
	}()
	wg.Wait()

	time.Sleep(time.Millisecond * 10)

	for {
		select {
		case data := <-conn.writeChan:
			for i := 0; i <= len(data)-7; i++ {
				if data[i] == cmdHeartRequest {
					sid := binary.BigEndian.Uint32(data[i+1 : i+5])
					length := binary.BigEndian.Uint16(data[i+5 : i+7])
					if sid != 0 {
						t.Errorf("expected stream ID 0 for heartbeat, got %d", sid)
					}
					if length != 0 {
						t.Errorf("expected length 0 for heartbeat, got %d", length)
					}
					return
				}
			}
		case <-time.After(time.Second):
			t.Fatal("did not receive heartbeat request frame")
		}
	}
}

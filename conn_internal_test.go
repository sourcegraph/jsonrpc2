package jsonrpc2

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestConnFailedWriteCleanup(t *testing.T) {
	tests := []struct {
		name string
		send func(*testing.T, *Conn) error
	}{
		{name: "dispatch", send: func(t *testing.T, c *Conn) error {
			w, err := c.DispatchCall(context.Background(), "failed", nil)
			if w.call != nil {
				t.Error("failed dispatch returned a waiter")
			}
			return err
		}},
		{name: "string ID", send: func(t *testing.T, c *Conn) error {
			_, err := c.DispatchCall(context.Background(), "failed", nil, StringID())
			return err
		}},
		{name: "custom ID", send: func(t *testing.T, c *Conn) error {
			_, err := c.DispatchCall(context.Background(), "failed", nil, PickID(ID{Num: 42}))
			return err
		}},
		{name: "custom string ID", send: func(t *testing.T, c *Conn) error {
			_, err := c.DispatchCall(context.Background(), "failed", nil, PickID(ID{Str: "failed", IsString: true}))
			return err
		}},
		{name: "call", send: func(t *testing.T, c *Conn) error {
			return c.Call(context.Background(), "failed", nil, nil)
		}},
		{name: "notification", send: func(t *testing.T, c *Conn) error {
			return c.Notify(context.Background(), "failed", nil)
		}},
		{name: "reply", send: func(t *testing.T, c *Conn) error {
			return c.Reply(context.Background(), ID{}, nil)
		}},
		{name: "error reply", send: func(t *testing.T, c *Conn) error {
			return c.ReplyWithError(context.Background(), ID{}, &Error{Code: CodeInternalError})
		}},
		{name: "response", send: func(t *testing.T, c *Conn) error {
			return c.SendResponse(context.Background(), &Response{ID: ID{}, Result: &jsonNull})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := &writeErrorStream{closed: make(chan struct{})}
			c := NewConn(context.Background(), stream, nil)
			defer c.Close()

			pending, err := c.DispatchCall(context.Background(), "pending", nil)
			if err != nil {
				t.Fatal(err)
			}
			stream.writeErr = errors.New("write failed")
			for i := 0; i < 3; i++ {
				if err := test.send(t, c); err != stream.writeErr {
					t.Fatalf("got error %v, want %v", err, stream.writeErr)
				}
				c.mu.Lock()
				count, original := len(c.pending), c.pending[ID{}]
				c.mu.Unlock()
				if count != 1 || original != pending.call {
					t.Errorf("after failure %d: got %d pending calls, want only the original call", i+1, count)
				}
			}

			stream.writeErr = nil
			next, err := c.DispatchCall(context.Background(), "next", nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			for _, waiter := range []Waiter{pending, next} {
				if err := waiter.Wait(context.Background(), nil); err != ErrClosed {
					t.Errorf("got error %v, want %v", err, ErrClosed)
				}
			}
			c.mu.Lock()
			count := len(c.pending)
			c.mu.Unlock()
			if count != 0 {
				t.Errorf("got %d pending calls after close, want 0", count)
			}
		})
	}
}

type writeErrorStream struct {
	writeErr error
	closed   chan struct{}
}

func (s *writeErrorStream) WriteObject(interface{}) error { return s.writeErr }

func (s *writeErrorStream) ReadObject(interface{}) error {
	<-s.closed
	return io.EOF
}

func (s *writeErrorStream) Close() error {
	close(s.closed)
	return nil
}

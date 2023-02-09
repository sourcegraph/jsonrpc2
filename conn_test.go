package jsonrpc2_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

func TestConn_DisconnectNotify(t *testing.T) {

	t.Run("EOF", func(t *testing.T) {
		connA, connB := net.Pipe()
		c := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewPlainObjectStream(connB), nil)
		// By closing connA, connB receives io.EOF
		if err := connA.Close(); err != nil {
			t.Error(err)
		}
		assertDisconnect(t, c, connB)
	})

	t.Run("Close", func(t *testing.T) {
		_, connB := net.Pipe()
		c := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewPlainObjectStream(connB), nil)
		if err := c.Close(); err != nil {
			t.Error(err)
		}
		assertDisconnect(t, c, connB)
	})

	t.Run("Close async", func(t *testing.T) {
		done := make(chan struct{})
		_, connB := net.Pipe()
		c := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewPlainObjectStream(connB), nil)
		go func() {
			if err := c.Close(); err != nil && err != jsonrpc2.ErrClosed {
				t.Error(err)
			}
			close(done)
		}()
		assertDisconnect(t, c, connB)
		<-done
	})

	t.Run("protocol error", func(t *testing.T) {
		connA, connB := net.Pipe()
		c := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewPlainObjectStream(connB), nil)
		connA.Write([]byte("invalid json"))
		assertDisconnect(t, c, connB)
	})
}

func TestConn_Close(t *testing.T) {
	t.Run("waiting for response", func(t *testing.T) {
		connA, connB := net.Pipe()
		nodeA := jsonrpc2.NewConn(
			context.Background(),
			jsonrpc2.NewPlainObjectStream(connA), noopHandler{},
		)
		defer nodeA.Close()
		nodeB := jsonrpc2.NewConn(
			context.Background(),
			jsonrpc2.NewPlainObjectStream(connB),
			noopHandler{},
		)
		defer nodeB.Close()

		ready := make(chan struct{})
		done := make(chan struct{})
		go func() {
			close(ready)
			err := nodeB.Call(context.Background(), "m", nil, nil)
			if err != jsonrpc2.ErrClosed {
				t.Errorf("got error %v, want %v", err, jsonrpc2.ErrClosed)
			}
			close(done)
		}()
		// Wait for the request to be sent before we close the connection.
		<-ready
		if err := nodeB.Close(); err != nil && err != jsonrpc2.ErrClosed {
			t.Error(err)
		}
		assertDisconnect(t, nodeB, connB)
		<-done
	})
}

func assertDisconnect(t *testing.T, c *jsonrpc2.Conn, conn io.Writer) {
	select {
	case <-c.DisconnectNotify():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no disconnect notification")
	}
	// Assert that conn is closed by trying to write to it.
	_, got := conn.Write(nil)
	want := io.ErrClosedPipe
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

package jsonrpc2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

var paramsTests = []struct {
	sendParams interface{}
	wantParams *json.RawMessage
}{
	{
		sendParams: nil,
		wantParams: nil,
	},
	{
		sendParams: jsonNull,
		wantParams: &jsonNull,
	},
	{
		sendParams: false,
		wantParams: rawJSONMessage("false"),
	},
	{
		sendParams: 0,
		wantParams: rawJSONMessage("0"),
	},
	{
		sendParams: "",
		wantParams: rawJSONMessage(`""`),
	},
	{
		sendParams: rawJSONMessage(`{"foo":"bar"}`),
		wantParams: rawJSONMessage(`{"foo":"bar"}`),
	},
}

func TestConn_DispatchCall(t *testing.T) {
	for _, test := range paramsTests {
		t.Run(fmt.Sprintf("%s", test.sendParams), func(t *testing.T) {
			testParams(t, test.wantParams, func(c *jsonrpc2.Conn) error {
				_, err := c.DispatchCall(context.Background(), "f", test.sendParams)
				return err
			})
		})
	}
}

func TestConn_Notify(t *testing.T) {
	for _, test := range paramsTests {
		t.Run(fmt.Sprintf("%s", test.sendParams), func(t *testing.T) {
			testParams(t, test.wantParams, func(c *jsonrpc2.Conn) error {
				return c.Notify(context.Background(), "f", test.sendParams)
			})
		})
	}
}

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
		c := jsonrpc2.NewConn(
			context.Background(),
			jsonrpc2.NewPlainObjectStream(connB),
			noopHandler{},
			// Suppress log message. This connection receives an invalid JSON
			// message that causes an error to be written to the logger. We
			// don't want this expected error to appear in os.Stderr though when
			// running tests in verbose mode or when other tests fail.
			jsonrpc2.SetLogger(log.New(io.Discard, "", 0)),
		)
		connA.Write([]byte("invalid json"))
		assertDisconnect(t, c, connB)
	})
}

func TestConn_Close(t *testing.T) {
	cases := []struct {
		name string
		run  func(*testing.T, context.Context, *jsonrpc2.Conn)
	}{{
		name: "during Call",
		run: func(t *testing.T, ctx context.Context, conn *jsonrpc2.Conn) {
			ready := make(chan struct{})
			done := make(chan struct{})
			go func() {
				close(ready)
				err := conn.Call(ctx, "m", nil, nil)
				if err != jsonrpc2.ErrClosed {
					t.Errorf("got error %v, want %v", err, jsonrpc2.ErrClosed)
				}
				close(done)
			}()
			// Wait for the request to be sent before we close the connection.
			<-ready
			if err := conn.Close(); err != nil && err != jsonrpc2.ErrClosed {
				t.Error(err)
			}
			<-done
		},
	}, {
		name: "during Wait",
		run: func(t *testing.T, ctx context.Context, conn *jsonrpc2.Conn) {
			call, err := conn.DispatchCall(ctx, "m", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			if err := call.Wait(ctx, nil); err != jsonrpc2.ErrClosed {
				t.Fatal(err)
			}
		},
	}, {
		name: "during Dispatch",
		run: func(t *testing.T, ctx context.Context, conn *jsonrpc2.Conn) {
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.DispatchCall(ctx, "m", nil, nil); err != jsonrpc2.ErrClosed {
				t.Fatal(err)
			}
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			connA, connB := net.Pipe()
			nodeA := jsonrpc2.NewConn(
				ctx,
				jsonrpc2.NewPlainObjectStream(connA), noopHandler{},
			)
			defer nodeA.Close()
			nodeB := jsonrpc2.NewConn(
				ctx,
				jsonrpc2.NewPlainObjectStream(connB),
				noopHandler{},
			)
			defer nodeB.Close()

			tc.run(t, ctx, nodeB)

			assertDisconnect(t, nodeB, connB)
		})
	}
}

func testParams(t *testing.T, want *json.RawMessage, fn func(c *jsonrpc2.Conn) error) {
	wg := &sync.WaitGroup{}
	handler := handlerFunc(func(ctx context.Context, conn *jsonrpc2.Conn, r *jsonrpc2.Request) {
		assertRawJSONMessage(t, r.Params, want)
		wg.Done()
	})

	client, server := newClientServer(handler)
	defer client.Close()
	defer server.Close()

	wg.Add(1)
	if err := fn(client); err != nil {
		t.Error(err)
	}
	wg.Wait()
}

func assertDisconnect(t *testing.T, c *jsonrpc2.Conn, conn io.Writer) {
	select {
	case <-c.DisconnectNotify():
	case <-time.After(200 * time.Millisecond):
		t.Error("no disconnect notification")
		return
	}
	// Assert that conn is closed by trying to write to it.
	_, got := conn.Write(nil)
	want := io.ErrClosedPipe
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func assertRawJSONMessage(t *testing.T, got *json.RawMessage, want *json.RawMessage) {
	// Assert pointers.
	if got == nil || want == nil {
		if got != want {
			t.Errorf("pointer: got %s, want %s", got, want)
		}
		return
	}
	{
		// If pointers are not nil, then assert values.
		got := string(*got)
		want := string(*want)
		if got != want {
			t.Errorf("value: got %q, want %q", got, want)
		}
	}
}

func newClientServer(handler jsonrpc2.Handler) (client *jsonrpc2.Conn, server *jsonrpc2.Conn) {
	ctx := context.Background()
	connA, connB := net.Pipe()
	client = jsonrpc2.NewConn(
		ctx,
		jsonrpc2.NewPlainObjectStream(connA),
		noopHandler{},
	)
	server = jsonrpc2.NewConn(
		ctx,
		jsonrpc2.NewPlainObjectStream(connB),
		handler,
	)
	return client, server
}

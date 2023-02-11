package jsonrpc2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
)

func TestError_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		setError func(err *jsonrpc2.Error)
		want     string
	}{
		{
			name: "Data == nil",
			want: `{"code":-32603,"message":"Internal error"}`,
		},
		{
			name: "Error.SetError(nil)",
			setError: func(err *jsonrpc2.Error) {
				err.SetError(nil)
			},
			want: `{"code":-32603,"message":"Internal error","data":null}`,
		},
		{
			name: "Error.SetError(0)",
			setError: func(err *jsonrpc2.Error) {
				err.SetError(0)
			},
			want: `{"code":-32603,"message":"Internal error","data":0}`,
		},
		{
			name: `Error.SetError("")`,
			setError: func(err *jsonrpc2.Error) {
				err.SetError("")
			},
			want: `{"code":-32603,"message":"Internal error","data":""}`,
		},
		{
			name: `Error.SetError(false)`,
			setError: func(err *jsonrpc2.Error) {
				err.SetError(false)
			},
			want: `{"code":-32603,"message":"Internal error","data":false}`,
		},
	}

	for _, test := range tests {
		e := &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInternalError,
			Message: "Internal error",
		}
		if test.setError != nil {
			test.setError(e)
		}
		b, err := json.Marshal(e)
		if err != nil {
			t.Error(err)
		}
		got := string(b)
		if got != test.want {
			t.Fatalf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}

// testHandlerA is the "server" handler.
type testHandlerA struct{ t *testing.T }

func (h *testHandlerA) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Notif {
		return // notification
	}
	if err := conn.Reply(ctx, req.ID, fmt.Sprintf("hello, #%s: %s", req.ID, *req.Params)); err != nil {
		h.t.Error(err)
	}

	if err := conn.Notify(ctx, "m", fmt.Sprintf("notif for #%s", req.ID)); err != nil {
		h.t.Error(err)
	}
}

// testHandlerB is the "client" handler.
type testHandlerB struct {
	t   *testing.T
	mu  sync.Mutex
	got []string
}

func (h *testHandlerB) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Notif {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.got = append(h.got, string(*req.Params))
		return
	}
	h.t.Errorf("testHandlerB got unexpected request %+v", req)
}

type streamMaker func(conn io.ReadWriteCloser) jsonrpc2.ObjectStream

func testClientServerForCodec(t *testing.T, streamMaker streamMaker) {
	ctx := context.Background()
	done := make(chan struct{})

	lis, err := net.Listen("tcp", "127.0.0.1:0") // any available address
	if err != nil {
		t.Fatal("Listen:", err)
	}
	defer func() {
		if lis == nil {
			return // already closed
		}
		if err = lis.Close(); err != nil {
			if !strings.HasSuffix(err.Error(), "use of closed network connection") {
				t.Fatal(err)
			}
		}
	}()

	ha := testHandlerA{t: t}
	go func() {
		if err = serve(ctx, lis, &ha, streamMaker); err != nil {
			if !strings.HasSuffix(err.Error(), "use of closed network connection") {
				t.Error(err)
			}
		}
		close(done)
	}()

	conn, err := net.Dial("tcp", lis.Addr().String())
	if err != nil {
		t.Fatal("Dial:", err)
	}
	testClientServer(ctx, t, streamMaker(conn))

	lis.Close()
	<-done // ensure Serve's error return (if any) is caught by this test
}

func TestClientServer(t *testing.T) {
	t.Run("tcp-varint-object-codec", func(t *testing.T) {
		testClientServerForCodec(t, func(conn io.ReadWriteCloser) jsonrpc2.ObjectStream {
			return jsonrpc2.NewBufferedStream(conn, jsonrpc2.VarintObjectCodec{})
		})
	})
	t.Run("tcp-vscode-object-codec", func(t *testing.T) {
		testClientServerForCodec(t, func(conn io.ReadWriteCloser) jsonrpc2.ObjectStream {
			return jsonrpc2.NewBufferedStream(conn, jsonrpc2.VSCodeObjectCodec{})
		})
	})
	t.Run("tcp-plain-object-codec", func(t *testing.T) {
		testClientServerForCodec(t, func(conn io.ReadWriteCloser) jsonrpc2.ObjectStream {
			return jsonrpc2.NewBufferedStream(conn, jsonrpc2.PlainObjectCodec{})
		})
	})
	t.Run("tcp-plain-object-stream", func(t *testing.T) {
		testClientServerForCodec(t, func(conn io.ReadWriteCloser) jsonrpc2.ObjectStream {
			return jsonrpc2.NewPlainObjectStream(conn)
		})
	})
	t.Run("websocket", func(t *testing.T) {
		ctx := context.Background()
		done := make(chan struct{})

		ha := testHandlerA{t: t}
		upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), &ha)
			<-jc.DisconnectNotify()
			close(done)
		}))
		defer s.Close()

		c, resp, err := websocket.DefaultDialer.Dial(strings.Replace(s.URL, "http:", "ws:", 1), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		defer c.Close()
		testClientServer(ctx, t, websocketjsonrpc2.NewObjectStream(c))

		<-done // keep the test running until the WebSocket disconnects (to avoid missing errors)
	})
}

func testClientServer(ctx context.Context, t *testing.T, stream jsonrpc2.ObjectStream) {
	hb := testHandlerB{t: t}
	cc := jsonrpc2.NewConn(ctx, stream, &hb)
	defer func() {
		if err := cc.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Simple
	const n = 100
	for i := 0; i < n; i++ {
		var got string
		if err := cc.Call(ctx, "f", []int32{1, 2, 3}, &got); err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("hello, #%d: [1,2,3]", i); got != want {
			t.Errorf("got result %q, want %q", got, want)
		}
	}
	time.Sleep(100 * time.Millisecond)
	hb.mu.Lock()
	got := hb.got
	hb.mu.Unlock()
	if len(got) != n {
		t.Errorf("testHandlerB got %d notifications, want %d", len(hb.got), n)
	}
	// Ensure messages are in order since we are not using the async handler.
	for i, s := range got {
		want := fmt.Sprintf(`"notif for #%d"`, i)
		if s != want {
			t.Fatalf("out of order response. got %q, want %q", s, want)
		}
	}
}

func inMemoryPeerConns() (io.ReadWriteCloser, io.ReadWriteCloser) {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	return &pipeReadWriteCloser{sr, sw}, &pipeReadWriteCloser{cr, cw}
}

type pipeReadWriteCloser struct {
	*io.PipeReader
	*io.PipeWriter
}

func (c *pipeReadWriteCloser) Close() error {
	err1 := c.PipeReader.Close()
	err2 := c.PipeWriter.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

type handlerFunc func(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request)

func (h handlerFunc) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	h(ctx, conn, req)
}

func TestHandlerBlocking(t *testing.T) {
	// We send N notifications with an increasing parameter. Since the
	// handler is blocking, we expect to process the notifications in the
	// order they are sent.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, b := inMemoryPeerConns()
	defer a.Close()
	defer b.Close()
	var (
		wg     sync.WaitGroup
		params []int
	)
	handler := handlerFunc(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
		var i int
		_ = json.Unmarshal(*req.Params, &i)
		// don't need to synchronize access to ids since we should be blocking
		params = append(params, i)
		wg.Done()
	})
	connA := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(a, jsonrpc2.VSCodeObjectCodec{}), handler)
	connB := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}), noopHandler{})
	defer connA.Close()
	defer connB.Close()

	const n = 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		if err := connB.Notify(ctx, "f", i); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if len(params) < n {
		t.Fatalf("want %d params, got %d", n, len(params))
	}
	for want, got := range params {
		if want != got {
			t.Fatalf("want param %d, got %d", want, got)
		}
	}
}

type noopHandler struct{}

func (noopHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {}

func serve(ctx context.Context, lis net.Listener, h jsonrpc2.Handler, streamMaker streamMaker, opts ...jsonrpc2.ConnOpt) error {
	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}
		jsonrpc2.NewConn(ctx, streamMaker(conn), h, opts...)
	}
}

func rawJSONMessage(v string) *json.RawMessage {
	b := []byte(v)
	return (*json.RawMessage)(&b)
}

var jsonNull = json.RawMessage("null")

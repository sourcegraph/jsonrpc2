package jsonrpc2_test

import (
	"bytes"
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

func TestRequest_MarshalJSON_jsonrpc(t *testing.T) {
	b, err := json.Marshal(&jsonrpc2.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if want := `"jsonrpc":"2.0"`; !strings.Contains(string(b), want) {
		t.Errorf("got %s, want it to include the string %s", b, want)
	}
}

func TestResponse_MarshalJSON_jsonrpc(t *testing.T) {
	b, err := json.Marshal(&jsonrpc2.Response{})
	if err != nil {
		t.Fatal(err)
	}
	if want := `"jsonrpc":"2.0"`; !strings.Contains(string(b), want) {
		t.Errorf("got %s, want it to include the string %s", b, want)
	}
}

func TestResponseMarshalJSON_Notif(t *testing.T) {
	tests := map[*jsonrpc2.Request]bool{
		&jsonrpc2.Request{ID: jsonrpc2.ID{Num: 0}}:                   true,
		&jsonrpc2.Request{ID: jsonrpc2.ID{Num: 1}}:                   true,
		&jsonrpc2.Request{ID: jsonrpc2.ID{Str: "", IsString: true}}:  true,
		&jsonrpc2.Request{ID: jsonrpc2.ID{Str: "a", IsString: true}}: true,
		&jsonrpc2.Request{Notif: true}:                               false,
	}
	for r, wantIDKey := range tests {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		hasIDKey := bytes.Contains(b, []byte(`"id"`))
		if hasIDKey != wantIDKey {
			t.Errorf("got %s, want contain id key: %v", b, wantIDKey)
		}
	}
}

func TestResponseUnmarshalJSON_Notif(t *testing.T) {
	tests := map[string]bool{
		`{"method":"f","id":0}`:   false,
		`{"method":"f","id":1}`:   false,
		`{"method":"f","id":"a"}`: false,
		`{"method":"f","id":""}`:  false,
		`{"method":"f"}`:          true,
	}
	for s, want := range tests {
		var r jsonrpc2.Request
		if err := json.Unmarshal([]byte(s), &r); err != nil {
			t.Fatal(err)
		}
		if r.Notif != want {
			t.Errorf("%s: got %v, want %v", s, r.Notif, want)
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

func TestClientServer(t *testing.T) {
	t.Run("tcp", func(t *testing.T) {
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
			if err := lis.Close(); err != nil {
				if !strings.HasSuffix(err.Error(), "use of closed network connection") {
					t.Fatal(err)
				}
			}
		}()

		ha := testHandlerA{t: t}
		go func() {
			if err := serve(ctx, lis, &ha); err != nil {
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
		testClientServer(ctx, t, jsonrpc2.NewBufferedStream(conn, jsonrpc2.VarintObjectCodec{}))

		lis.Close()
		<-done // ensure Serve's error return (if any) is caught by this test
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

		c, _, err := websocket.DefaultDialer.Dial(strings.Replace(s.URL, "http:", "ws:", 1), nil)
		if err != nil {
			t.Fatal(err)
		}
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
	if len(hb.got) != n {
		t.Errorf("testHandlerB got %d notifications, want %d", len(hb.got), n)
	}
	hb.mu.Unlock()
}

type noopHandler struct{}

func (noopHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {}

type readWriteCloser struct {
	read, write func(p []byte) (n int, err error)
}

func (x readWriteCloser) Read(p []byte) (n int, err error) {
	return x.read(p)
}

func (x readWriteCloser) Write(p []byte) (n int, err error) {
	return x.write(p)
}

func (readWriteCloser) Close() error { return nil }

func eof(p []byte) (n int, err error) {
	return 0, io.EOF
}

func TestConn_DisconnectNotify_EOF(t *testing.T) {
	c := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(&readWriteCloser{eof, eof}, jsonrpc2.VarintObjectCodec{}), nil)
	select {
	case <-c.DisconnectNotify():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no disconnect notification")
	}
}

func TestConn_DisconnectNotify_Close(t *testing.T) {
	c := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(&readWriteCloser{eof, eof}, jsonrpc2.VarintObjectCodec{}), nil)
	if err := c.Close(); err != nil {
		t.Error(err)
	}
	select {
	case <-c.DisconnectNotify():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no disconnect notification")
	}
}

func TestConn_DisconnectNotify_Close_async(t *testing.T) {
	done := make(chan struct{})
	c := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(&readWriteCloser{eof, eof}, jsonrpc2.VarintObjectCodec{}), nil)
	go func() {
		if err := c.Close(); err != nil && err != jsonrpc2.ErrClosed {
			t.Error(err)
		}
		close(done)
	}()
	select {
	case <-c.DisconnectNotify():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no disconnect notification")
	}
	<-done
}

func TestConn_Close_waitingForResponse(t *testing.T) {
	c := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(&readWriteCloser{eof, eof}, jsonrpc2.VarintObjectCodec{}), noopHandler{})
	done := make(chan struct{})
	go func() {
		if err := c.Call(context.Background(), "m", nil, nil); err != jsonrpc2.ErrClosed {
			t.Errorf("got error %v, want %v", err, jsonrpc2.ErrClosed)
		}
		close(done)
	}()
	if err := c.Close(); err != nil && err != jsonrpc2.ErrClosed {
		t.Error(err)
	}
	select {
	case <-c.DisconnectNotify():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no disconnect notification")
	}
	<-done
}

func serve(ctx context.Context, lis net.Listener, h jsonrpc2.Handler, opt ...jsonrpc2.ConnOpt) error {
	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}
		jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(conn, jsonrpc2.VarintObjectCodec{}), h, opt...)
	}
}

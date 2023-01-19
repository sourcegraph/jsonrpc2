package jsonrpc2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestPickID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, b := inMemoryPeerConns()
	defer a.Close()
	defer b.Close()

	handler := handlerFunc(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
		if err := conn.Reply(ctx, req.ID, fmt.Sprintf("hello, #%s: %s", req.ID, *req.Params)); err != nil {
			t.Error(err)
		}
	})
	connA := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(a, jsonrpc2.VSCodeObjectCodec{}), handler)
	connB := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}), noopHandler{})
	defer connA.Close()
	defer connB.Close()

	const n = 100
	for i := 0; i < n; i++ {
		var opts []jsonrpc2.CallOption
		id := jsonrpc2.ID{Num: uint64(i)}

		// This is the actual test, every 3rd request we specify the
		// ID and ensure we get a response with the correct ID echoed
		// back
		if i%3 == 0 {
			id = jsonrpc2.ID{
				Str:      fmt.Sprintf("helloworld-%d", i/3),
				IsString: true,
			}
			opts = append(opts, jsonrpc2.PickID(id))
		}

		var got string
		if err := connB.Call(ctx, "f", []int32{1, 2, 3}, &got, opts...); err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("hello, #%s: [1,2,3]", id); got != want {
			t.Errorf("got result %q, want %q", got, want)
		}
	}
}

func TestStringID(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, b := inMemoryPeerConns()
	defer a.Close()
	defer b.Close()

	handler := handlerFunc(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
		replyWithError := func(msg string) {
			respErr := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidRequest, Message: msg}
			if err := conn.ReplyWithError(ctx, req.ID, respErr); err != nil {
				t.Error(err)
			}
		}
		if !req.ID.IsString {
			replyWithError("ID.IsString should be true")
			return
		}
		if len(req.ID.Str) == 0 {
			replyWithError("ID.Str should be populated but is empty")
			return
		}
		if err := conn.Reply(ctx, req.ID, "ok"); err != nil {
			t.Error(err)
		}
	})
	connA := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(a, jsonrpc2.VSCodeObjectCodec{}), handler)
	connB := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}), noopHandler{})
	defer connA.Close()
	defer connB.Close()

	var res string
	if err := connB.Call(ctx, "f", nil, &res, jsonrpc2.StringID()); err != nil {
		t.Fatal(err)
	}
}

func TestExtraField(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, b := inMemoryPeerConns()
	defer a.Close()
	defer b.Close()

	handler := handlerFunc(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
		replyWithError := func(msg string) {
			respErr := &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidRequest, Message: msg}
			if err := conn.ReplyWithError(ctx, req.ID, respErr); err != nil {
				t.Error(err)
			}
		}
		var sessionID string
		for _, field := range req.ExtraFields {
			if field.Name != "sessionId" {
				continue
			}
			var ok bool
			sessionID, ok = field.Value.(string)
			if !ok {
				t.Errorf("\"sessionId\" is not a string: %v", field.Value)
			}
		}
		if sessionID == "" {
			replyWithError("sessionId must be set")
			return
		}
		if sessionID != "session" {
			replyWithError("sessionId has the wrong value")
			return
		}
		if err := conn.Reply(ctx, req.ID, "ok"); err != nil {
			t.Error(err)
		}
	})
	connA := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(a, jsonrpc2.VSCodeObjectCodec{}), handler)
	connB := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}), noopHandler{})
	defer connA.Close()
	defer connB.Close()

	var res string
	if err := connB.Call(ctx, "f", nil, &res, jsonrpc2.ExtraField("sessionId", "session")); err != nil {
		t.Fatal(err)
	}
}

func TestOmitNilParams(t *testing.T) {
	rawJSONMessage := func(v string) *json.RawMessage {
		b := []byte(v)
		return (*json.RawMessage)(&b)
	}

	type testCase struct {
		callOpt    jsonrpc2.CallOption
		sendParams interface{}
		wantParams *json.RawMessage
	}

	testCases := []testCase{
		{
			sendParams: nil,
			wantParams: rawJSONMessage("null"),
		},
		{
			sendParams: rawJSONMessage("null"),
			wantParams: rawJSONMessage("null"),
		},
		{
			callOpt:    jsonrpc2.OmitNilParams(),
			sendParams: nil,
			wantParams: nil,
		},
		{
			callOpt:    jsonrpc2.OmitNilParams(),
			sendParams: rawJSONMessage("null"),
			wantParams: rawJSONMessage("null"),
		},
	}

	assert := func(got *json.RawMessage, want *json.RawMessage) error {
		// Assert pointers.
		if got == nil || want == nil {
			if got != want {
				return fmt.Errorf("got %v, want %v", got, want)
			}
			return nil
		}
		{
			// If pointers are not nil, then assert values.
			got := string(*got)
			want := string(*want)
			if got != want {
				return fmt.Errorf("got %q, want %q", got, want)
			}
		}
		return nil
	}

	newClientServer := func(handler jsonrpc2.Handler) (client *jsonrpc2.Conn, server *jsonrpc2.Conn) {
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

	for i, tc := range testCases {

		t.Run(fmt.Sprintf("test case %v", i), func(t *testing.T) {
			t.Run("call", func(t *testing.T) {
				handler := jsonrpc2.HandlerWithError(func(ctx context.Context, c *jsonrpc2.Conn, r *jsonrpc2.Request) (result interface{}, err error) {
					return nil, assert(r.Params, tc.wantParams)
				})

				client, server := newClientServer(handler)
				defer client.Close()
				defer server.Close()

				if err := client.Call(context.Background(), "f", tc.sendParams, nil, tc.callOpt); err != nil {
					t.Fatal(err)
				}
			})
			t.Run("notify", func(t *testing.T) {
				wg := &sync.WaitGroup{}
				handler := handlerFunc(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
					err := assert(req.Params, tc.wantParams)
					if err != nil {
						t.Error(err)
					}
					wg.Done()
				})

				client, server := newClientServer(handler)
				defer client.Close()
				defer server.Close()

				wg.Add(1)
				if err := client.Notify(context.Background(), "f", tc.sendParams, tc.callOpt); err != nil {
					t.Fatal(err)
				}
				wg.Wait()
			})
		})
	}
}

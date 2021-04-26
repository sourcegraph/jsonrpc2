package jsonrpc2_test

import (
	"context"
	"fmt"
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

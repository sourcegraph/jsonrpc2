package jsonrpc2_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestSetLogger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rd, wr := io.Pipe()
	defer rd.Close()
	defer wr.Close()

	buf := bufio.NewReader(rd)
	logger := log.New(wr, "", log.Lmsgprefix)

	a, b := net.Pipe()
	connA := jsonrpc2.NewConn(
		ctx,
		jsonrpc2.NewBufferedStream(a, jsonrpc2.VSCodeObjectCodec{}),
		noopHandler{},
		jsonrpc2.SetLogger(logger),
	)
	connB := jsonrpc2.NewConn(
		ctx,
		jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}),
		noopHandler{},
	)
	defer connA.Close()
	defer connB.Close()

	// Write a response with no corresponding request.
	if err := connB.Reply(ctx, jsonrpc2.ID{Num: 0}, nil); err != nil {
		t.Fatal(err)
	}

	want := "jsonrpc2: ignoring response #0 with no corresponding request\n"
	got, err := buf.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestOmitNilParams(t *testing.T) {
	rawJSONMessage := func(v string) *json.RawMessage {
		b := []byte(v)
		return (*json.RawMessage)(&b)
	}

	type testCase struct {
		connOpt    jsonrpc2.ConnOpt
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
			connOpt:    jsonrpc2.OmitNilParams(),
			sendParams: nil,
			wantParams: nil,
		},
		{
			connOpt:    jsonrpc2.OmitNilParams(),
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

	newClientServer := func(handler jsonrpc2.Handler, connOpt jsonrpc2.ConnOpt) (client *jsonrpc2.Conn, server *jsonrpc2.Conn) {
		ctx := context.Background()
		connA, connB := net.Pipe()
		client = jsonrpc2.NewConn(
			ctx,
			jsonrpc2.NewPlainObjectStream(connA),
			noopHandler{},
			connOpt,
		)
		server = jsonrpc2.NewConn(
			ctx,
			jsonrpc2.NewPlainObjectStream(connB),
			handler,
			connOpt,
		)
		return client, server
	}

	for i, tc := range testCases {

		t.Run(fmt.Sprintf("test case %v", i), func(t *testing.T) {
			t.Run("call", func(t *testing.T) {
				handler := jsonrpc2.HandlerWithError(func(ctx context.Context, c *jsonrpc2.Conn, r *jsonrpc2.Request) (result interface{}, err error) {
					return nil, assert(r.Params, tc.wantParams)
				})

				client, server := newClientServer(handler, tc.connOpt)
				defer client.Close()
				defer server.Close()

				if err := client.Call(context.Background(), "f", tc.sendParams, nil); err != nil {
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

				client, server := newClientServer(handler, tc.connOpt)
				defer client.Close()
				defer server.Close()

				wg.Add(1)
				if err := client.Notify(context.Background(), "f", tc.sendParams); err != nil {
					t.Fatal(err)
				}
				wg.Wait()
			})
		})
	}
}

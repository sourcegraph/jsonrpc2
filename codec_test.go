package jsonrpc2_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestVarintObjectCodec(t *testing.T) {
	want := 789
	var buf bytes.Buffer
	if err := (jsonrpc2.VarintObjectCodec{}).WriteObject(&buf, want); err != nil {
		t.Fatal(err)
	}
	var v int
	if err := (jsonrpc2.VarintObjectCodec{}).ReadObject(bufio.NewReader(&buf), &v); err != nil {
		t.Fatal(err)
	}
	if want := want; v != want {
		t.Errorf("got %v, want %v", v, want)
	}
}

func TestVSCodeObjectCodec_ReadObject(t *testing.T) {
	s := "Content-Type: foo\r\nContent-Length: 123\r\n\r\n789"
	var v int
	if err := (jsonrpc2.VSCodeObjectCodec{}).ReadObject(bufio.NewReader(strings.NewReader(s)), &v); err != nil {
		t.Fatal(err)
	}
	if want := 789; v != want {
		t.Errorf("got %v, want %v", v, want)
	}
}

func TestPlainObjectCodec(t *testing.T) {
	type Message struct {
		One   string
		Two   uint
		Three bool
	}

	cA, cB := net.Pipe()
	connA := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(cA, jsonrpc2.PlainObjectCodec{}),
		noopHandler{},
	)
	defer connA.Close()

	// echoHandler unmarshals the request's params object and echos the object
	// back as the response's result.
	var echoHandler handlerFunc = func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
		msg := &Message{}
		if err := json.Unmarshal(*req.Params, msg); err != nil {
			conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidRequest, Message: err.Error()})
			return
		}
		conn.Reply(ctx, req.ID, msg)
	}
	connB := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(cB, jsonrpc2.PlainObjectCodec{}),
		echoHandler,
	)
	defer connB.Close()

	req := &Message{One: "hello world", Two: 123, Three: true}
	res := &Message{}
	if err := connA.Call(context.Background(), "f", req, res); err != nil {
		t.Fatal(err)
	}

	if got, want := res, req; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", res, req)
	}
}

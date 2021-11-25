package jsonrpc2_test

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
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
		jsonrpc2.NoopHandler{},
		jsonrpc2.SetLogger(logger),
	)
	connB := jsonrpc2.NewConn(
		ctx,
		jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}),
		jsonrpc2.NoopHandler{},
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

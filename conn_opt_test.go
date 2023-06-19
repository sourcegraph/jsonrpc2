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

type dummyHandler struct {
	t *testing.T
}

func (h *dummyHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if !req.Notif {
		err := conn.Reply(ctx, req.ID, nil)
		if err != nil {
			h.t.Error(err)
			return
		}
	}
}

func TestLogMessages(t *testing.T) {
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
		&dummyHandler{t},
		jsonrpc2.LogMessages(logger),
	)
	connB := jsonrpc2.NewConn(
		ctx,
		jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}),
		&dummyHandler{t},
	)
	defer connA.Close()
	defer connB.Close()

	go func() {
		if err := connA.Call(ctx, "method1", nil, nil); err != nil {
			t.Error(err)
			return
		}
		if err := connB.Call(ctx, "method2", nil, nil); err != nil {
			t.Error(err)
			return
		}
		if err := connA.Notify(ctx, "notification1", nil); err != nil {
			t.Error(err)
			return
		}
		if err := connB.Notify(ctx, "notification2", nil); err != nil {
			t.Error(err)
			return
		}
	}()

	for i, want := range []string{
		"jsonrpc2: <-- request #0: method1: null\n",
		"jsonrpc2: --> result #0: method1: null\n",
		"jsonrpc2: --> request #0: method2: null\n",
		"jsonrpc2: <-- result #0: method2: null\n",
		"jsonrpc2: <-- notif: notification1: null\n",
		"jsonrpc2: --> notif: notification2: null\n",
	} {
		got, err := buf.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("message %v: got %q, want %q", i, got, want)
		}
	}
}

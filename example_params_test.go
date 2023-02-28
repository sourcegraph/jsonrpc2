package jsonrpc2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/sourcegraph/jsonrpc2"
)

// Send a JSON-RPC notification with its params member omitted.
func ExampleConn_Notify_paramsOmitted() {
	ctx := context.Background()

	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()

	rpcConn := jsonrpc2.NewConn(ctx, jsonrpc2.NewPlainObjectStream(connA), nil)

	// Send the JSON-RPC notification.
	go func() {
		// Set params to nil.
		if err := rpcConn.Notify(ctx, "foo", nil); err != nil {
			fmt.Fprintln(os.Stderr, "notify:", err)
		}
	}()

	// Read the raw JSON-RPC notification on connB.
	//
	// Reading the raw JSON-RPC request is for the purpose of this example only.
	// Use a jsonrpc2.Handler to read parsed requests.
	buf := make([]byte, 64)
	n, err := connB.Read(buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
	}

	fmt.Printf("%s\n", buf[:n])

	// Output: {"jsonrpc":"2.0","method":"foo"}
}

// Send a JSON-RPC notification with its params member set to null.
func ExampleConn_Notify_nullParams() {
	ctx := context.Background()

	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()

	rpcConn := jsonrpc2.NewConn(ctx, jsonrpc2.NewPlainObjectStream(connA), nil)

	// Send the JSON-RPC notification.
	go func() {
		// Set params to the JSON null value.
		params := json.RawMessage("null")
		if err := rpcConn.Notify(ctx, "foo", params); err != nil {
			fmt.Fprintln(os.Stderr, "notify:", err)
		}
	}()

	// Read the raw JSON-RPC notification on connB.
	//
	// Reading the raw JSON-RPC request is for the purpose of this example only.
	// Use a jsonrpc2.Handler to read parsed requests.
	buf := make([]byte, 64)
	n, err := connB.Read(buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
	}

	fmt.Printf("%s\n", buf[:n])

	// Output: {"jsonrpc":"2.0","method":"foo","params":null}
}

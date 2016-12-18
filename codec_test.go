package jsonrpc2

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestVarintObjectCodec(t *testing.T) {
	want := 789
	var buf bytes.Buffer
	if err := (VarintObjectCodec{}).WriteObject(&buf, want); err != nil {
		t.Fatal(err)
	}
	var v int
	if err := (VarintObjectCodec{}).ReadObject(bufio.NewReader(&buf), &v); err != nil {
		t.Fatal(err)
	}
	if want := want; v != want {
		t.Errorf("got %v, want %v", v, want)
	}
}

func TestVSCodeObjectCodec_ReadObject(t *testing.T) {
	s := "Content-Type: foo\r\nContent-Length: 123\r\n\r\n789"
	var v int
	if err := (VSCodeObjectCodec{}).ReadObject(bufio.NewReader(strings.NewReader(s)), &v); err != nil {
		t.Fatal(err)
	}
	if want := 789; v != want {
		t.Errorf("got %v, want %v", v, want)
	}
}

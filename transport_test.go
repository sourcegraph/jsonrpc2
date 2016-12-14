package jsonrpc2

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"strings"
	"testing"
)

func TestVarintObjectCodec(t *testing.T) {
	want := "123456789"

	var buf bytes.Buffer
	if err := (VarintObjectCodec{}).WriteObject(&buf, []byte(want)); err != nil {
		t.Fatal(err)
	}
	r, err := VarintObjectCodec{}.NextObjectReader(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("got %q, want %q", data, want)
	}
}

func TestVSCodeObjectCodec_NextObjectReader(t *testing.T) {
	s := "Content-Type: foo\r\nContent-Length: 123\r\n\r\n{}"
	r, err := VSCodeObjectCodec{}.NextObjectReader(bufio.NewReader(strings.NewReader(s)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{}"; string(b) != want {
		t.Errorf("got %q, want %q", b, want)
	}
}

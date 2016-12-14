package jsonrpc2

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
)

// A Transport sends and receives JSON-RPC 2.0 objects.
type Transport interface {
	SendObject(obj []byte) error
	ReadObject() ([]byte, error)
	io.Closer
}

// A streamTransport is a Transport that uses a bidirectional byte
// stream to send and receive objects.
type streamTransport struct {
	conn io.Closer // all writes should go through w, all reads through r
	w    *bufio.Writer
	r    *bufio.Reader

	objectCodec ObjectCodec
}

// NewStreamTransport creates a new Transport from a network
// connection (or other similar interface). The codec is used to write
// JSON-RPC 2.0 objects to the stream.
func NewStreamTransport(conn io.ReadWriteCloser, codec ObjectCodec) Transport {
	return streamTransport{
		conn:        conn,
		w:           bufio.NewWriter(conn),
		r:           bufio.NewReader(conn),
		objectCodec: codec,
	}
}

// SendObject implements Transport.
func (t streamTransport) SendObject(obj []byte) error {
	if err := t.objectCodec.WriteObject(t.w, obj); err != nil {
		return err
	}
	return t.w.Flush()
}

// ReadObject implements Transport.
func (t streamTransport) ReadObject() ([]byte, error) {
	r, err := t.objectCodec.GetObjectReader(t.r)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(r)
}

// Close implements Transport.
func (t streamTransport) Close() error {
	return t.conn.Close()
}

// An ObjectCodec specifies how to write a JSON-RPC 2.0 object to the
// stream.
type ObjectCodec interface {
	// WriteObject writes a JSON-RPC 2.0 object to the stream.
	WriteObject(stream io.Writer, obj []byte) error

	// GetObjectReader returns a reader that, when read to EOF,
	// contains only the bytes of the next JSON-RPC 2.0 object.
	//
	// Typically the returned io.Reader is an io.LimitReader based on
	// the stream (whose max length is determined by a header that
	// prefixes objects in the stream, for example).
	GetObjectReader(stream *bufio.Reader) (io.Reader, error)
}

// VarintObjectCodec reads/writes JSON-RPC 2.0 objects
// with a varint header that encodes the byte length.
type VarintObjectCodec struct{}

// WriteObject implements ObjectCodec.
func (VarintObjectCodec) WriteObject(stream io.Writer, obj []byte) error {
	var buf [binary.MaxVarintLen64]byte
	b := binary.PutUvarint(buf[:], uint64(len(obj)))
	if _, err := stream.Write(buf[:b]); err != nil {
		return err
	}
	if _, err := stream.Write(obj); err != nil {
		return err
	}
	return nil
}

// GetObjectReader implements ObjectCodec.
func (VarintObjectCodec) GetObjectReader(stream *bufio.Reader) (io.Reader, error) {
	b, err := binary.ReadUvarint(stream)
	if err != nil {
		return nil, err
	}
	return io.LimitReader(stream, int64(b)), nil
}

// VSCodeObjectCodec reads/writes JSON-RPC 2.0 objects with
// Content-Length and Content-Type headers, as specified by
// https://github.com/Microsoft/language-server-protocol/blob/master/protocol.md#base-protocol.
type VSCodeObjectCodec struct{}

// WriteObject implements ObjectCodec.
func (VSCodeObjectCodec) WriteObject(stream io.Writer, obj []byte) error {
	if _, err := fmt.Fprintf(stream, "Content-Length: %d\r\n", len(obj)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(stream, "Content-Type: application/vscode-jsonrpc; charset=utf8\r\n\r\n"); err != nil {
		return err
	}
	if _, err := stream.Write(obj); err != nil {
		return err
	}
	return nil
}

// GetObjectReader implements ObjectCodec.
func (VSCodeObjectCodec) GetObjectReader(stream *bufio.Reader) (io.Reader, error) {
	var contentLength uint64
	for {
		line, err := stream.ReadString('\r')
		if err != nil {
			return nil, err
		}
		b, err := stream.ReadByte()
		if err != nil {
			return nil, err
		}
		if b != '\n' {
			return nil, fmt.Errorf(`jsonrpc2: line endings must be \r\n`)
		}
		if line == "\r" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			line = strings.TrimPrefix(line, "Content-Length: ")
			line = strings.TrimSpace(line)
			var err error
			contentLength, err = strconv.ParseUint(line, 10, 32)
			if err != nil {
				return nil, err
			}
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("jsonrpc2: no Content-Length header found")
	}
	return io.LimitReader(stream, int64(contentLength)), nil
}

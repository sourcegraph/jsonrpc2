// Package websocket provides a WebSocket-based transport for JSON-RPC
// 2.0.
package websocket

import (
	"io"

	"github.com/gorilla/websocket"
)

// A Transport is a jsonrpc2.Transport that uses a WebSocket to send
// and receive JSON-RPC 2.0 objects.
type Transport struct {
	conn *websocket.Conn
}

// NewTransport creates a new jsonrpc2.Transport for sending and
// receiving JSON-RPC 2.0 objects over a WebSocket.
func NewTransport(conn *websocket.Conn) Transport {
	return Transport{conn: conn}
}

// SendObject implements jsonrpc2.Transport.
func (t Transport) SendObject(obj []byte) error {
	return t.conn.WriteMessage(websocket.BinaryMessage, obj)
}

// NextObjectReader implements jsonrpc2.Transport.
func (t Transport) NextObjectReader() (io.Reader, error) {
	_, mr, err := t.conn.NextReader()
	if e, ok := err.(*websocket.CloseError); ok {
		if e.Code == websocket.CloseAbnormalClosure && e.Text == io.ErrUnexpectedEOF.Error() {
			// Suppress a noisy (but harmless) log message by
			// unwrapping this error.
			err = io.ErrUnexpectedEOF
		}
	}
	return mr, err
}

// Close implements jsonrpc2.Transport.
func (t Transport) Close() error {
	return t.conn.Close()
}

package jsonrpc2

// Handler handles JSON-RPC requests and notifications.
type Handler interface {
	// Handle is called to handle a request. No other requests are handled
	// until it returns. If you do not require strict ordering behavior
	// of received RPCs, it is suggested to wrap your handler in
	// AsyncHandler.
	Handle(*Conn, *Request)
}

type HandlerFunc func(*Conn, *Request)

func (f HandlerFunc) Handle(conn *Conn, req *Request) {
	f(conn, req)
}

type Middleware func(Handler) Handler

type chain struct {
	ms []Middleware
}

func Chain(middleware ...Middleware) chain {
	return chain{ms: append([]Middleware(nil), middleware...)}
}

func (c chain) Then(h Handler) Handler {
	if h == nil {
		panic("nil handler")
	}

	for i := range c.ms {
		h = c.ms[len(c.ms)-1-i](h)
	}

	return h
}

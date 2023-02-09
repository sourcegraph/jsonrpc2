package jsonrpc2

import (
	"encoding/json"
	"testing"
)

func TestAnyMessage(t *testing.T) {
	tests := map[string]struct {
		request, response, invalid bool
	}{
		// Single messages
		`{}`:                                   {invalid: true},
		`{"foo":"bar"}`:                        {invalid: true},
		`{"method":"m"}`:                       {request: true},
		`{"result":123}`:                       {response: true},
		`{"result":null}`:                      {response: true},
		`{"error":{"code":456,"message":"m"}}`: {response: true},
	}
	for s, want := range tests {
		var m anyMessage
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			if !want.invalid {
				t.Errorf("%s: error: %v", s, err)
			}
			continue
		}
		if (m.request != nil) != want.request {
			t.Errorf("%s: got request %v, want %v", s, m.request != nil, want.request)
		}
		if (m.response != nil) != want.response {
			t.Errorf("%s: got response %v, want %v", s, m.response != nil, want.response)
		}
	}
}

package jsonrpc2

import (
	"encoding/json"
	"reflect"
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
				t.Errorf("%s: error: %s", s, err)
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

func TestMessageCodec(t *testing.T) {
	obj := json.RawMessage(`{"foo":"bar"}`)
	tests := []struct {
		v, vempty interface{}
	}{
		{
			v:      &Request{ID: ID{Num: 123}},
			vempty: &Request{ID: ID{Num: 123}},
		},
		{
			v:      &Response{ID: ID{Num: 123}, Result: &obj},
			vempty: &Response{ID: ID{Num: 123}, Result: &obj},
		},
	}
	for _, test := range tests {
		b, err := json.Marshal(test.v)
		if err != nil {
			t.Fatal(err)
		}

		if err := json.Unmarshal(b, test.vempty); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(test.vempty, test.v) {
			t.Errorf("got %+v, want %+v", test.vempty, test.v)
		}
	}
}

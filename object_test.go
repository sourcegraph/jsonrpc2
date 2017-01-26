package jsonrpc2

import (
	"bytes"
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

func TestRequest_MarshalUnmarshalJSON(t *testing.T) {
	null := json.RawMessage("null")
	obj := json.RawMessage(`{"foo":"bar"}`)
	tests := []struct {
		data []byte
		want Request
	}{
		{
			data: []byte(`{"method":"m","params":{"foo":"bar"},"id":123,"jsonrpc":"2.0"}`),
			want: Request{ID: ID{Num: 123}, Method: "m", Params: &obj},
		},
		{
			data: []byte(`{"method":"m","params":null,"id":123,"jsonrpc":"2.0"}`),
			want: Request{ID: ID{Num: 123}, Method: "m", Params: &null},
		},
		{
			data: []byte(`{"method":"m","id":123,"jsonrpc":"2.0"}`),
			want: Request{ID: ID{Num: 123}, Method: "m", Params: nil},
		},
	}
	for _, test := range tests {
		var got Request
		if err := json.Unmarshal(test.data, &got); err != nil {
			t.Error(err)
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q: got %+v, want %+v", test.data, got, test.want)
			continue
		}
		data, err := json.Marshal(got)
		if err != nil {
			t.Error(err)
			continue
		}
		if !bytes.Equal(data, test.data) {
			t.Errorf("got JSON %q, want %q", data, test.data)
		}
	}
}

func TestResponse_MarshalUnmarshalJSON(t *testing.T) {
	null := json.RawMessage("null")
	obj := json.RawMessage(`{"foo":"bar"}`)
	tests := []struct {
		data  []byte
		want  Response
		error bool
	}{
		{
			data: []byte(`{"id":123,"result":{"foo":"bar"},"jsonrpc":"2.0"}`),
			want: Response{ID: ID{Num: 123}, Result: &obj},
		},
		{
			data: []byte(`{"id":123,"result":null,"jsonrpc":"2.0"}`),
			want: Response{ID: ID{Num: 123}, Result: &null},
		},
		{
			data:  []byte(`{"id":123,"jsonrpc":"2.0"}`),
			want:  Response{ID: ID{Num: 123}, Result: nil},
			error: true, // either result or error field must be set
		},
	}
	for _, test := range tests {
		var got Response
		if err := json.Unmarshal(test.data, &got); err != nil {
			t.Error(err)
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q: got %+v, want %+v", test.data, got, test.want)
			continue
		}
		data, err := json.Marshal(got)
		if err != nil {
			if test.error {
				continue
			}
			t.Error(err)
			continue
		}
		if test.error {
			t.Errorf("%q: expected error", test.data)
			continue
		}
		if !bytes.Equal(data, test.data) {
			t.Errorf("got JSON %q, want %q", data, test.data)
		}
	}
}

package jsonrpc2_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestRequest_MarshalJSON_jsonrpc(t *testing.T) {
	b, err := json.Marshal(&jsonrpc2.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":0,"jsonrpc":"2.0","method":""}`; string(b) != want {
		t.Errorf("got %q, want %q", b, want)
	}
}

func TestRequest_MarshalUnmarshalJSON(t *testing.T) {
	obj := json.RawMessage(`{"foo":"bar"}`)
	tests := []struct {
		data []byte
		want jsonrpc2.Request
	}{
		{
			data: []byte(`{"id":123,"jsonrpc":"2.0","method":"m","params":{"foo":"bar"}}`),
			want: jsonrpc2.Request{ID: jsonrpc2.ID{Num: 123}, Method: "m", Params: &obj},
		},
		{
			data: []byte(`{"id":123,"jsonrpc":"2.0","method":"m","params":null}`),
			want: jsonrpc2.Request{ID: jsonrpc2.ID{Num: 123}, Method: "m", Params: &jsonNull},
		},
		{
			data: []byte(`{"id":123,"jsonrpc":"2.0","method":"m"}`),
			want: jsonrpc2.Request{ID: jsonrpc2.ID{Num: 123}, Method: "m", Params: nil},
		},
		{
			data: []byte(`{"id":123,"jsonrpc":"2.0","method":"m","sessionId":"session"}`),
			want: jsonrpc2.Request{ID: jsonrpc2.ID{Num: 123}, Method: "m", Params: nil, ExtraFields: []jsonrpc2.RequestField{{Name: "sessionId", Value: "session"}}},
		},
	}
	for _, test := range tests {
		var got jsonrpc2.Request
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

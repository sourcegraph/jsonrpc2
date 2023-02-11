package jsonrpc2_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestResponse_MarshalJSON_jsonrpc(t *testing.T) {
	b, err := json.Marshal(&jsonrpc2.Response{Result: &jsonNull})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":0,"result":null,"jsonrpc":"2.0"}`; string(b) != want {
		t.Errorf("got %q, want %q", b, want)
	}
}

func TestResponseMarshalJSON_Notif(t *testing.T) {
	tests := map[*jsonrpc2.Request]bool{
		{ID: jsonrpc2.ID{Num: 0}}:                   true,
		{ID: jsonrpc2.ID{Num: 1}}:                   true,
		{ID: jsonrpc2.ID{Str: "", IsString: true}}:  true,
		{ID: jsonrpc2.ID{Str: "a", IsString: true}}: true,
		{Notif: true}: false,
	}
	for r, wantIDKey := range tests {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		hasIDKey := bytes.Contains(b, []byte(`"id"`))
		if hasIDKey != wantIDKey {
			t.Errorf("got %s, want contain id key: %v", b, wantIDKey)
		}
	}
}

func TestResponseUnmarshalJSON_Notif(t *testing.T) {
	tests := map[string]bool{
		`{"method":"f","id":0}`:   false,
		`{"method":"f","id":1}`:   false,
		`{"method":"f","id":"a"}`: false,
		`{"method":"f","id":""}`:  false,
		`{"method":"f"}`:          true,
	}
	for s, want := range tests {
		var r jsonrpc2.Request
		if err := json.Unmarshal([]byte(s), &r); err != nil {
			t.Fatal(err)
		}
		if r.Notif != want {
			t.Errorf("%s: got %v, want %v", s, r.Notif, want)
		}
	}
}

func TestResponse_MarshalUnmarshalJSON(t *testing.T) {
	obj := json.RawMessage(`{"foo":"bar"}`)
	tests := []struct {
		data  []byte
		want  jsonrpc2.Response
		error bool
	}{
		{
			data: []byte(`{"id":123,"result":{"foo":"bar"},"jsonrpc":"2.0"}`),
			want: jsonrpc2.Response{ID: jsonrpc2.ID{Num: 123}, Result: &obj},
		},
		{
			data: []byte(`{"id":123,"result":null,"jsonrpc":"2.0"}`),
			want: jsonrpc2.Response{ID: jsonrpc2.ID{Num: 123}, Result: &jsonNull},
		},
		{
			data:  []byte(`{"id":123,"jsonrpc":"2.0"}`),
			want:  jsonrpc2.Response{ID: jsonrpc2.ID{Num: 123}, Result: nil},
			error: true, // either result or error field must be set
		},
	}
	for _, test := range tests {
		var got jsonrpc2.Response
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

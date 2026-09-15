package mcpserver

import (
	"encoding/json"
	"testing"
)

func TestPrimitiveQueryValueCanonicalizesIntegerJSONTokens(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{name: "decimal token", raw: json.RawMessage(`25.0`), want: "25"},
		{name: "exponent token", raw: json.RawMessage(`25e0`), want: "25"},
		{name: "negative zero", raw: json.RawMessage(`-0.0`), want: "0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := primitiveQueryValue("integer", test.raw)
			if err != nil {
				t.Fatalf("primitiveQueryValue() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("primitiveQueryValue() = %q, want %q", got, test.want)
			}
		})
	}
}

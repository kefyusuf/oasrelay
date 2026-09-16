package mcpserver

import (
	"encoding/json"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func TestBindQueryParameterPreservesExistingRawQuery(t *testing.T) {
	parameter := &oasopenapi.QueryParameter{Name: "limit", Type: "integer"}

	got, err := bindQueryParameter(
		"https://example.test/customers?token=a;b",
		parameter,
		map[string]json.RawMessage{"limit": json.RawMessage(`25`)},
	)
	if err != nil {
		t.Fatalf("bindQueryParameter() error = %v", err)
	}

	want := "https://example.test/customers?token=a;b&limit=25"
	if got != want {
		t.Fatalf("bindQueryParameter() = %q, want %q", got, want)
	}
}

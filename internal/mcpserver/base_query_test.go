package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func TestBindQueryParameterPreservesExistingRawQuery(t *testing.T) {
	input := map[string]json.RawMessage{
		"limit": json.RawMessage(`25`),
	}
	parameter := &oasopenapi.QueryParameter{Name: "limit", Type: "integer"}

	got, err := bindQueryParameter(
		"https://example.test/customers?token=a;b&mode=raw%2Fvalue",
		parameter,
		input,
	)
	if err != nil {
		t.Fatalf("bindQueryParameter() error = %v", err)
	}

	wantQuery := "token=a;b&mode=raw%2Fvalue&limit=25"
	if !strings.HasSuffix(got, "?"+wantQuery) {
		t.Fatalf("bound endpoint = %q, want raw query suffix %q", got, wantQuery)
	}
}

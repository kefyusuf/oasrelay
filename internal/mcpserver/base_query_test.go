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

func TestBindQueryParametersPreservesEndpointWhenAllOptionalQueriesAreOmitted(t *testing.T) {
	endpoint := "https://example.test/customers?token=a;b&mode=raw%2Fvalue"
	parameters := []oasopenapi.QueryParameter{
		{Name: "limit", Type: "integer", Optional: true},
		{Name: "filter", Type: "string", Optional: true},
	}

	got, err := bindQueryParameters(endpoint, parameters, map[string]json.RawMessage{})
	if err != nil {
		t.Fatalf("bindQueryParameters() error = %v", err)
	}
	if got != endpoint {
		t.Fatalf("bound endpoint = %q, want unchanged %q", got, endpoint)
	}
}


package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExecutesInheritedParametersWithOperationOverride(t *testing.T) {
	var calls atomic.Int32
	requests := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		requests <- r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer upstream.Close()
	document, err := os.ReadFile("../../testdata/inherited-parameters.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte(strings.Replace(string(document), "https://example.test", upstream.URL, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	operation, err := oasopenapi.SelectGET(path, "getCustomerOrders")
	if err != nil {
		t.Fatalf("SelectGET: %v", err)
	}
	server, err := New(operation, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	session, cleanup := connectClient(t, server)
	defer cleanup()
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) != 1 {
		t.Fatalf("ListTools=%#v error=%v", listed, err)
	}
	encoded, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema["required"], []any{"customerId"}) || schema["additionalProperties"] != false {
		t.Fatalf("input schema=%#v", schema)
	}
	properties := schema["properties"].(map[string]any)
	if properties["limit"].(map[string]any)["type"] != "integer" || properties["customerId"].(map[string]any)["type"] != "string" {
		t.Fatalf("properties=%#v", properties)
	}
	for _, input := range []map[string]any{
		{}, {"customerId": nil}, {"customerId": 25},
		{"customerId": "a/b", "limit": nil},
		{"customerId": "a/b", "limit": "25"},
		{"customerId": "a/b", "extra": true},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: operation.OperationID, Arguments: input})
		if err != nil || !result.IsError || calls.Load() != 0 {
			t.Fatalf("invalid input=%#v result=%#v error=%v upstream calls=%d", input, result, err, calls.Load())
		}
	}
	for _, test := range []struct {
		input map[string]any
		uri   string
	}{
		{map[string]any{"customerId": "a/b"}, "/api/customers/a%2Fb/orders?token=a;b"},
		{map[string]any{"customerId": "a/b", "limit": json.Number("25e0")}, "/api/customers/a%2Fb/orders?token=a;b&limit=25"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: operation.OperationID, Arguments: test.input})
		if err != nil || result.IsError {
			t.Fatalf("CallTool result=%#v error=%v", result, err)
		}
		if got := <-requests; got != test.uri {
			t.Fatalf("request=%q want=%q", got, test.uri)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls=%d want=2", calls.Load())
	}
}

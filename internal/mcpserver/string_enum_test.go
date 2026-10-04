package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStringEnumSchemaAndCalls(t *testing.T) {
	var calls atomic.Int32
	requests := make(chan string, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		requests <- r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	operation := oasopenapi.SelectedOperation{
		OperationID: "enumItems", Endpoint: upstream.URL + "/items/{state}?fixed=a%20b",
		PathParameters:  []oasopenapi.PathParameter{{Name: "state", Type: "string", Enum: []string{"a/b", "..", "{state}"}}},
		QueryParameters: []oasopenapi.QueryParameter{{Name: "filter", Type: "string", Optional: true, Enum: []string{"", "Active", " active "}}},
	}
	server, err := New(operation, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	session, cleanup := connectClient(t, server)
	defer cleanup()
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Type string
			Enum []string
		}
		Required             []string
		AdditionalProperties bool
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema.Properties["state"].Enum, operation.PathParameters[0].Enum) || !reflect.DeepEqual(schema.Properties["filter"].Enum, operation.QueryParameters[0].Enum) || !reflect.DeepEqual(schema.Required, []string{"state"}) || schema.AdditionalProperties {
		t.Fatalf("schema = %s", data)
	}
	for _, arguments := range []map[string]any{
		{"state": "A/B"}, {"state": "a/b", "filter": "active"},
		{"state": "a/b", "filter": nil}, {"state": "a/b", "filter": true},
		{"state": "a/b", "unknown": "Active"}, {},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "enumItems", Arguments: arguments})
		if err != nil || !result.IsError {
			t.Fatalf("invalid %v: result = %#v, error = %v", arguments, result, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid calls reached upstream: %d", calls.Load())
	}
	for _, test := range []struct {
		arguments map[string]any
		uri       string
	}{
		{map[string]any{"state": "a/b"}, "/items/a%2Fb?fixed=a%20b"},
		{map[string]any{"state": "..", "filter": ""}, "/items/%2E%2E?fixed=a%20b&filter="},
		{map[string]any{"state": "{state}", "filter": " active "}, "/items/%7Bstate%7D?fixed=a%20b&filter=+active+"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "enumItems", Arguments: test.arguments})
		if err != nil || result.IsError {
			t.Fatalf("valid call: %#v, %v", result, err)
		}
		if uri := <-requests; uri != test.uri {
			t.Fatalf("URI = %q, want %q", uri, test.uri)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("upstream calls = %d", calls.Load())
	}
}

func TestStringEnumBinderEnforcesExactMembership(t *testing.T) {
	for _, location := range []string{"path", "query"} {
		t.Run(location, func(t *testing.T) {
			op := oasopenapi.SelectedOperation{Endpoint: "https://example.test/items/{state}"}
			values := []string{"", "Active", " a/b ", "ş%"}
			if location == "path" {
				op.PathParameters = []oasopenapi.PathParameter{{Name: "state", Type: "string", Enum: values}}
			} else {
				op.Endpoint = "https://example.test/items"
				op.QueryParameters = []oasopenapi.QueryParameter{{Name: "state", Type: "string", Enum: values}}
			}
			for _, raw := range []string{`"active"`, `" Active "`, `"a/b"`, `null`, `42`, `true`} {
				if _, err := bindOperationArguments(op, map[string]json.RawMessage{"state": json.RawMessage(raw)}); err == nil {
					t.Fatalf("accepted %s", raw)
				}
			}
			for _, value := range values {
				raw, _ := json.Marshal(value)
				if _, err := bindOperationArguments(op, map[string]json.RawMessage{"state": raw}); err != nil {
					t.Fatalf("rejected %q: %v", value, err)
				}
			}
		})
	}
}

func TestNewRejectsInvalidStringEnumMetadata(t *testing.T) {
	for _, test := range []struct {
		name, kind string
		values     []string
	}{
		{"empty", "string", []string{}}, {"duplicate", "string", []string{"a", "a"}},
		{"integer", "integer", []string{"1"}}, {"boolean", "boolean", []string{"true"}},
	} {
		for _, location := range []string{"path", "query"} {
			t.Run(test.name+"_"+location, func(t *testing.T) {
				op := oasopenapi.SelectedOperation{OperationID: "enumItems", Endpoint: "https://example.test/items/{state}"}
				if location == "path" {
					op.PathParameters = []oasopenapi.PathParameter{{Name: "state", Type: test.kind, Enum: test.values}}
				} else {
					op.QueryParameters = []oasopenapi.QueryParameter{{Name: "state", Type: test.kind, Enum: test.values}}
				}
				if _, err := New(op, http.DefaultClient); err == nil {
					t.Fatal("accepted malformed enum metadata")
				}
			})
		}
	}
}

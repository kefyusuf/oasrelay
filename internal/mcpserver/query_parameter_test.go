package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExposesRequiredPrimitiveQueryParameterSchema(t *testing.T) {
	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    "http://example.test/customers",
		QueryParameters: []oasopenapi.QueryParameter{{
			Name: "limit",
			Type: "integer",
		}},
	}, http.DefaultClient)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(listed.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(listed.Tools))
	}

	encoded, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}

	if schema["type"] != "object" {
		t.Fatalf("input schema = %s, want object type", encoded)
	}
	additionalProperties, ok := schema["additionalProperties"].(bool)
	if !ok || additionalProperties {
		t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
	}
	required, ok := schema["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "limit" {
		t.Fatalf("required = %#v, want [limit]", schema["required"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", schema["properties"])
	}
	limit, ok := properties["limit"].(map[string]any)
	if !ok || limit["type"] != "integer" {
		t.Fatalf("limit schema = %#v, want integer", properties["limit"])
	}
}

func TestServerExposesOptionalPrimitiveQueryInputSchema(t *testing.T) {
	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    "http://example.test/customers",
		QueryParameters: []oasopenapi.QueryParameter{{
			Name:     "limit",
			Type:     "integer",
			Optional: true,
		}},
	}, http.DefaultClient)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(listed.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(listed.Tools))
	}

	encoded, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", schema["properties"])
	}
	limit, ok := properties["limit"].(map[string]any)
	if !ok || limit["type"] != "integer" {
		t.Fatalf("limit schema = %#v", properties["limit"])
	}
	if _, exists := schema["required"]; exists {
		t.Fatalf("required = %#v, want field omitted", schema["required"])
	}
	if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
		t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
	}
}

func TestServerBindsPrimitiveQueryParameter(t *testing.T) {
	tests := []struct {
		name       string
		paramType  string
		argument   any
		wantValue  string
		wantEscape string
	}{
		{
			name:       "string",
			paramType:  "string",
			argument:   "a b&c",
			wantValue:  "a b&c",
			wantEscape: "filter=a+b%26c",
		},
		{name: "integer", paramType: "integer", argument: 25, wantValue: "25", wantEscape: "filter=25"},
		{
			name:       "integer decimal token",
			paramType:  "integer",
			argument:   json.Number("25.0"),
			wantValue:  "25",
			wantEscape: "filter=25",
		},
		{
			name:       "integer exponent token",
			paramType:  "integer",
			argument:   json.Number("25e0"),
			wantValue:  "25",
			wantEscape: "filter=25",
		},
		{name: "number", paramType: "number", argument: 1.25, wantValue: "1.25", wantEscape: "filter=1.25"},
		{name: "boolean", paramType: "boolean", argument: true, wantValue: "true", wantEscape: "filter=true"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			requestURL := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				requestURL <- request.URL.String()
				if got := request.URL.Query().Get("filter"); got != test.wantValue {
					t.Errorf("query filter = %q, want %q", got, test.wantValue)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(oasopenapi.SelectedOperation{
				OperationID: "listCustomers",
				Method:      http.MethodGet,
				Path:        "/customers",
				Endpoint:    upstream.URL + "/customers",
				QueryParameters: []oasopenapi.QueryParameter{{
					Name: "filter",
					Type: test.paramType,
				}},
			}, upstream.Client())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "listCustomers",
				Arguments: map[string]any{"filter": test.argument},
			})
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if result.IsError {
				t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
			}
			if calls.Load() != 1 {
				t.Fatalf("upstream calls = %d, want 1", calls.Load())
			}

			gotURL := <-requestURL
			if !strings.Contains(gotURL, test.wantEscape) {
				t.Fatalf("request URL = %q, want encoded query %q", gotURL, test.wantEscape)
			}
		})
	}
}

func TestServerBindsOptionalQueryWhenOmittedOrSupplied(t *testing.T) {
	var calls atomic.Int32
	requests := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		requests <- request.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    upstream.URL + "/customers?token=a;b&mode=raw%2Fvalue",
		QueryParameters: []oasopenapi.QueryParameter{{
			Name:     "limit",
			Type:     "integer",
			Optional: true,
		}},
	}, upstream.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "listCustomers",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool() omitted optional query error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() omitted optional query IsError = true; content = %#v", result.Content)
	}
	if got := <-requests; got != "/customers?token=a;b&mode=raw%2Fvalue" {
		t.Fatalf("omitted optional query RequestURI = %q", got)
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "listCustomers",
		Arguments: map[string]any{
			"limit": json.Number("25e0"),
		},
	})
	if err != nil {
		t.Fatalf("CallTool() supplied optional query error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() supplied optional query IsError = true; content = %#v", result.Content)
	}
	if got := <-requests; got != "/customers?token=a;b&mode=raw%2Fvalue&limit=25" {
		t.Fatalf("supplied optional query RequestURI = %q", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls.Load())
	}
}

func TestServerRejectsInvalidOptionalQueryArgumentsBeforeUpstream(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "explicit null", arguments: map[string]any{"limit": nil}},
		{name: "wrong primitive type", arguments: map[string]any{"limit": "25"}},
		{name: "unknown field", arguments: map[string]any{"cursor": "next"}},
		{name: "optional plus unknown field", arguments: map[string]any{"limit": 25, "cursor": "next"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(oasopenapi.SelectedOperation{
				OperationID: "listCustomers",
				Method:      http.MethodGet,
				Path:        "/customers",
				Endpoint:    upstream.URL + "/customers",
				QueryParameters: []oasopenapi.QueryParameter{{
					Name:     "limit",
					Type:     "integer",
					Optional: true,
				}},
			}, upstream.Client())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "listCustomers",
				Arguments: test.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() protocol error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("CallTool() IsError = false, want true; result = %#v", result)
			}
			if calls.Load() != 0 {
				t.Fatalf("upstream calls = %d, want 0", calls.Load())
			}
		})
	}
}

func TestServerRejectsInvalidQueryArgumentsBeforeUpstream(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "missing required parameter", arguments: map[string]any{}},
		{name: "wrong primitive type", arguments: map[string]any{"limit": "25"}},
		{name: "unknown field", arguments: map[string]any{"limit": 25, "cursor": "next"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(oasopenapi.SelectedOperation{
				OperationID: "listCustomers",
				Method:      http.MethodGet,
				Path:        "/customers",
				Endpoint:    upstream.URL + "/customers",
				QueryParameters: []oasopenapi.QueryParameter{{
					Name: "limit",
					Type: "integer",
				}},
			}, upstream.Client())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "listCustomers",
				Arguments: test.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() protocol error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("CallTool() IsError = false, want true; result = %#v", result)
			}
			if calls.Load() != 0 {
				t.Fatalf("upstream calls = %d, want 0", calls.Load())
			}
		})
	}
}

package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExposesRequiredPrimitivePathParameterSchema(t *testing.T) {
	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "getCustomer",
		Method:      http.MethodGet,
		Path:        "/customers/{customerId}",
		Endpoint:    "http://example.test/customers/%7BcustomerId%7D",
		PathParameter: &oasopenapi.PathParameter{
			Name: "customerId",
			Type: "string",
		},
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

	required, ok := schema["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "customerId" {
		t.Fatalf("required = %#v, want [customerId]", schema["required"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", schema["properties"])
	}
	customerID, ok := properties["customerId"].(map[string]any)
	if !ok || customerID["type"] != "string" {
		t.Fatalf("customerId schema = %#v, want string", properties["customerId"])
	}
}

func TestServerBindsPrimitivePathParameter(t *testing.T) {
	tests := []struct {
		name            string
		parameterType   string
		argument        any
		wantPathSegment string
	}{
		{name: "string with slash", parameterType: "string", argument: "a/b", wantPathSegment: "a%2Fb"},
		{name: "dot segment", parameterType: "string", argument: ".", wantPathSegment: "%2E"},
		{name: "parent dot segment", parameterType: "string", argument: "..", wantPathSegment: "%2E%2E"},
		{name: "integer", parameterType: "integer", argument: json.Number("25.0"), wantPathSegment: "25"},
		{name: "number", parameterType: "number", argument: 1.25, wantPathSegment: "1.25"},
		{name: "boolean", parameterType: "boolean", argument: true, wantPathSegment: "true"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestURI := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requestURI <- request.RequestURI
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(oasopenapi.SelectedOperation{
				OperationID: "getCustomer",
				Method:      http.MethodGet,
				Path:        "/customers/{customerId}",
				Endpoint:    upstream.URL + "/api/customers/%7BcustomerId%7D?token=a;b",
				PathParameter: &oasopenapi.PathParameter{
					Name: "customerId",
					Type: test.parameterType,
				},
			}, upstream.Client())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "getCustomer",
				Arguments: map[string]any{"customerId": test.argument},
			})
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if result.IsError {
				t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
			}

			want := "/api/customers/" + test.wantPathSegment + "?token=a;b"
			if got := <-requestURI; got != want {
				t.Fatalf("RequestURI = %q, want %q", got, want)
			}
		})
	}
}

func TestServerRejectsInvalidPathArgumentsBeforeUpstream(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "missing required parameter", arguments: map[string]any{}},
		{name: "explicit null", arguments: map[string]any{"customerId": nil}},
		{name: "wrong primitive type", arguments: map[string]any{"customerId": 25}},
		{name: "unknown field", arguments: map[string]any{"customerId": "cus_1", "extra": true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(oasopenapi.SelectedOperation{
				OperationID: "getCustomer",
				Method:      http.MethodGet,
				Path:        "/customers/{customerId}",
				Endpoint:    upstream.URL + "/customers/%7BcustomerId%7D",
				PathParameter: &oasopenapi.PathParameter{
					Name: "customerId",
					Type: "string",
				},
			}, upstream.Client())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "getCustomer",
				Arguments: test.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() protocol error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("CallTool() IsError = false, want true; result = %#v", result)
			}
			if calls != 0 {
				t.Fatalf("upstream calls = %d, want 0", calls)
			}
		})
	}
}

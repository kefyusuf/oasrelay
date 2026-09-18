package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func combinedOperation(endpoint string) oasopenapi.SelectedOperation {
	return oasopenapi.SelectedOperation{
		OperationID: "getCustomerOrders",
		Method:      http.MethodGet,
		Path:        "/customers/{customerId}/orders",
		Endpoint:    endpoint,
		PathParameter: &oasopenapi.PathParameter{
			Name: "customerId",
			Type: "string",
		},
		QueryParameter: &oasopenapi.QueryParameter{
			Name: "limit",
			Type: "integer",
		},
	}
}

func TestServerExposesCombinedPathAndQueryInputSchema(t *testing.T) {
	server, err := New(
		combinedOperation("http://example.test/customers/%7BcustomerId%7D/orders"),
		http.DefaultClient,
	)
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

	if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
		t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
	}

	required, ok := schema["required"].([]any)
	if !ok ||
		len(required) != 2 ||
		required[0] != "customerId" ||
		required[1] != "limit" {
		t.Fatalf("required = %#v, want [customerId limit]", schema["required"])
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", schema["properties"])
	}
	customerID, ok := properties["customerId"].(map[string]any)
	if !ok || customerID["type"] != "string" {
		t.Fatalf("customerId schema = %#v", properties["customerId"])
	}
	limit, ok := properties["limit"].(map[string]any)
	if !ok || limit["type"] != "integer" {
		t.Fatalf("limit schema = %#v", properties["limit"])
	}
}

func TestServerRejectsCombinedParametersWithSameInputName(t *testing.T) {
	operation := combinedOperation(
		"http://example.test/customers/%7Bid%7D/orders",
	)
	operation.PathParameter.Name = "id"
	operation.QueryParameter.Name = "id"

	_, err := New(operation, http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "share MCP input name") {
		t.Fatalf("New() error = %v, want duplicate input-name rejection", err)
	}
}

func TestServerBindsPathThenQueryAndPreservesRawServerQuery(t *testing.T) {
	requests := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests <- request.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	operation := combinedOperation(
		upstream.URL + "/api/customers/%7BcustomerId%7D/orders?token=a;b",
	)

	server, err := New(operation, upstream.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "getCustomerOrders",
		Arguments: map[string]any{
			"customerId": "a/b",
			"limit":      json.Number("25e0"),
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
	}

	if got := <-requests; got != "/api/customers/a%2Fb/orders?token=a;b&limit=25" {
		t.Fatalf("RequestURI = %q", got)
	}
}

func TestServerRejectsInvalidCombinedArgumentsBeforeUpstream(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
	}{
		{
			name:      "missing path parameter",
			arguments: map[string]any{"limit": 25},
		},
		{
			name:      "missing query parameter",
			arguments: map[string]any{"customerId": "cus_123"},
		},
		{
			name: "null path parameter",
			arguments: map[string]any{
				"customerId": nil,
				"limit":      25,
			},
		},
		{
			name: "null query parameter",
			arguments: map[string]any{
				"customerId": "cus_123",
				"limit":      nil,
			},
		},
		{
			name: "wrong path type",
			arguments: map[string]any{
				"customerId": 123,
				"limit":      25,
			},
		},
		{
			name: "wrong query type",
			arguments: map[string]any{
				"customerId": "cus_123",
				"limit":      "25",
			},
		},
		{
			name: "extra field",
			arguments: map[string]any{
				"customerId": "cus_123",
				"limit":      25,
				"extra":      true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(
				combinedOperation(
					upstream.URL + "/customers/%7BcustomerId%7D/orders",
				),
				upstream.Client(),
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "getCustomerOrders",
				Arguments: test.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() protocol error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("CallTool() IsError = false, want true")
			}
			if calls != 0 {
				t.Fatalf("upstream calls = %d, want 0", calls)
			}
		})
	}
}

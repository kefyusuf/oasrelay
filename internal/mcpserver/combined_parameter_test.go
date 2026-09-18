package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
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

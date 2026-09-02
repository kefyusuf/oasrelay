package openapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectParameterlessGETUsesDocumentServer(t *testing.T) {
	got, err := SelectParameterlessGET(fixturePath("single-get-api.yaml"), "listCustomers")
	if err != nil {
		t.Fatalf("SelectParameterlessGET() error = %v", err)
	}

	want := SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Summary:     "List customers",
		Description: "Returns the current customer collection.",
		Endpoint:    "https://document.example.test/api/customers",
	}
	if got != want {
		t.Fatalf("SelectedOperation = %#v, want %#v", got, want)
	}
}

func TestSelectParameterlessGETUsesNarrowestServer(t *testing.T) {
	tests := []struct {
		name        string
		operationID string
		endpoint    string
	}{
		{
			name:        "path server overrides document server",
			operationID: "pathScoped",
			endpoint:    "https://path.example.test/v1/path-scoped",
		},
		{
			name:        "operation server overrides path server",
			operationID: "operationScoped",
			endpoint:    "https://operation.example.test/v2/operation-scoped",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SelectParameterlessGET(fixturePath("single-get-api.yaml"), test.operationID)
			if err != nil {
				t.Fatalf("SelectParameterlessGET() error = %v", err)
			}
			if got.Endpoint != test.endpoint {
				t.Fatalf("Endpoint = %q, want %q", got.Endpoint, test.endpoint)
			}
		})
	}
}

func TestSelectParameterlessGETRejectsUnsupportedOperations(t *testing.T) {
	tests := []struct {
		name        string
		operationID string
		message     string
	}{
		{
			name:        "missing operation",
			operationID: "missing",
			message:     `operationId "missing" was not found`,
		},
		{
			name:        "known non-GET operation",
			operationID: "createCustomer",
			message:     `operationId "createCustomer" uses POST; this version supports GET only`,
		},
		{
			name:        "operation parameters",
			operationID: "getCustomer",
			message:     `operationId "getCustomer" has parameters; this version supports parameterless operations only`,
		},
		{
			name:        "path parameters",
			operationID: "listFiltered",
			message:     `operationId "listFiltered" has parameters; this version supports parameterless operations only`,
		},
		{
			name:        "request body",
			operationID: "getWithBody",
			message:     `operationId "getWithBody" has a request body; this version does not support request bodies`,
		},
		{
			name:        "relative server",
			operationID: "relativeServer",
			message:     `operationId "relativeServer": has no usable absolute HTTP(S) server URL`,
		},
		{
			name:        "variable server",
			operationID: "variableServer",
			message:     `operationId "variableServer": uses server variables; this version supports static server URLs only`,
		},
		{
			name:        "unsupported scheme",
			operationID: "ftpServer",
			message:     `operationId "ftpServer": has no usable absolute HTTP(S) server URL`,
		},
		{
			name:        "invalid MCP tool name",
			operationID: "invalid tool",
			message:     `operationId "invalid tool" is not a valid MCP tool name`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := SelectParameterlessGET(fixturePath("single-get-api.yaml"), test.operationID)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want substring %q", err, test.message)
			}
		})
	}
}

func TestSelectParameterlessGETRejectsMissingServer(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: No Server API
  version: 1.0.0
paths:
  /health:
    get:
      operationId: health
      responses:
        "200":
          description: Healthy
`)

	_, err := SelectParameterlessGET(path, "health")
	if err == nil || !strings.Contains(err.Error(), `operationId "health": has no usable absolute HTTP(S) server URL`) {
		t.Fatalf("error = %v, want missing-server context", err)
	}
}

func TestSelectParameterlessGETRejectsOverlongToolName(t *testing.T) {
	operationID := strings.Repeat("a", 129)
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Long Name API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /health:
    get:
      operationId: `+operationID+`
      responses:
        "200":
          description: Healthy
`)

	_, err := SelectParameterlessGET(path, operationID)
	if err == nil || !strings.Contains(err.Error(), "is not a valid MCP tool name") {
		t.Fatalf("error = %v, want tool-name context", err)
	}
}

func writeSelectionSpec(t *testing.T, document string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("write OpenAPI document: %v", err)
	}
	return path
}

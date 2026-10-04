package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func TestRunServeForwardsTwoRequiredPrimitivePathParameters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	document := `openapi: 3.0.3
info:
  title: Two Path API
  version: 1.0.0
servers:
  - url: https://example.test/api
paths:
  /customers/{customerId}/orders/{orderId}:
    get:
      operationId: getOrder
      parameters:
        - name: orderId
          in: path
          required: true
          schema:
            type: integer
        - name: customerId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Order
`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("write OpenAPI fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	var selected oasopenapi.SelectedOperation
	calls := 0
	code := runWithServe([]string{"serve", "--operation-id", "getOrder", path}, &stdout, &stderr,
		func(_ context.Context, operation oasopenapi.SelectedOperation) error {
			calls++
			selected = operation
			return nil
		})
	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 || calls != 1 {
		t.Fatalf("code = %d; stdout = %q; stderr = %q; calls = %d", code, stdout.String(), stderr.String(), calls)
	}
	want := []oasopenapi.PathParameter{{Name: "orderId", Type: "integer"}, {Name: "customerId", Type: "string"}}
	if !reflect.DeepEqual(selected.PathParameters, want) || len(selected.QueryParameters) != 0 {
		t.Fatalf("selected parameters = %#v, queries = %#v", selected.PathParameters, selected.QueryParameters)
	}
	if selected.OperationID != "getOrder" || selected.Method != "GET" || selected.Endpoint != "https://example.test/api/customers/%7BcustomerId%7D/orders/%7BorderId%7D" {
		t.Fatalf("selected operation = %#v", selected)
	}
}

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func TestRunServeAcceptsOneRequiredPrimitivePathParameter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	document := `openapi: 3.0.3
info:
  title: Path API
  version: 1.0.0
servers:
  - url: https://example.test/api
paths:
  /customers/{customerId}:
    get:
      operationId: getCustomer
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Customer
`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("write OpenAPI fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var selected oasopenapi.SelectedOperation
	calls := 0

	code := runWithServe(
		[]string{"serve", "--operation-id", "getCustomer", path},
		&stdout,
		&stderr,
		func(_ context.Context, operation oasopenapi.SelectedOperation) error {
			calls++
			selected = operation
			return nil
		},
	)

	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 || calls != 1 {
		t.Fatalf(
			"code = %d; stdout = %q; stderr = %q; calls = %d",
			code,
			stdout.String(),
			stderr.String(),
			calls,
		)
	}
	if selected.QueryParameter != nil {
		t.Fatalf("selected QueryParameter = %#v, want nil", selected.QueryParameter)
	}
	if selected.PathParameter == nil ||
		selected.PathParameter.Name != "customerId" ||
		selected.PathParameter.Type != "string" {
		t.Fatalf("selected PathParameter = %#v", selected.PathParameter)
	}
	if selected.Endpoint != "https://example.test/api/customers/%7BcustomerId%7D" {
		t.Fatalf("selected Endpoint = %q", selected.Endpoint)
	}
}

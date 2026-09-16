package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func TestRunServeAcceptsOneRequiredPrimitiveQueryParameter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	document := `openapi: 3.0.3
info:
  title: Query API
  version: 1.0.0
servers:
  - url: https://example.test/api
paths:
  /customers:
    get:
      operationId: listCustomers
      parameters:
        - name: limit
          in: query
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: Customer collection
`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("write OpenAPI fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var selected oasopenapi.SelectedOperation
	calls := 0

	code := runWithServe(
		[]string{"serve", "--operation-id", "listCustomers", path},
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
	if selected.QueryParameter == nil ||
		selected.QueryParameter.Name != "limit" ||
		selected.QueryParameter.Type != "integer" {
		t.Fatalf("selected QueryParameter = %#v", selected.QueryParameter)
	}
}

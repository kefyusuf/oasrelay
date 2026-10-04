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

func TestRunServeForwardsStringEnumMetadataWithoutStdoutPollution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	document := `openapi: 3.0.3
info:
  title: String Enum API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /items/{state}:
    get:
      operationId: getItems
      parameters:
        - name: state
          in: path
          required: true
          schema:
            type: string
            enum: [active, " active "]
        - name: filter
          in: query
          schema:
            type: string
            enum: ["", all]
      responses:
        '200':
          description: Items
`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	var selected oasopenapi.SelectedOperation
	calls := 0
	code := runWithServe([]string{"serve", "--operation-id", "getItems", path}, &stdout, &stderr,
		func(_ context.Context, operation oasopenapi.SelectedOperation) error {
			calls++
			selected = operation
			return nil
		})
	if code != 0 || calls != 1 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("code = %d; calls = %d; stdout = %q; stderr = %q", code, calls, stdout.String(), stderr.String())
	}
	wantPath := []oasopenapi.PathParameter{{Name: "state", Type: "string", Enum: []string{"active", " active "}}}
	wantQuery := []oasopenapi.QueryParameter{{Name: "filter", Type: "string", Optional: true, Enum: []string{"", "all"}}}
	if !reflect.DeepEqual(selected.PathParameters, wantPath) || !reflect.DeepEqual(selected.QueryParameters, wantQuery) {
		t.Fatalf("selected enum metadata = %#v", selected)
	}
}

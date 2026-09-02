package openapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

func TestInspectFileLoadsYAMLAndSortsGETOperations(t *testing.T) {
	got, err := InspectFile(fixturePath("customer-api.yaml"))
	if err != nil {
		t.Fatalf("InspectFile() error = %v", err)
	}

	if got.OpenAPIVersion != "3.0.3" || got.Title != "Customer API" ||
		got.APIVersion != "1.0.0" || got.ServerURL != "http://localhost:8080" {
		t.Fatalf("Inspection = %#v", got)
	}

	want := []Operation{
		{OperationID: "listCustomers", Method: "GET", Path: "/customers"},
		{OperationID: "getCustomer", Method: "GET", Path: "/customers/{customerId}"},
	}
	if len(got.Operations) != len(want) {
		t.Fatalf("len(Operations) = %d, want %d", len(got.Operations), len(want))
	}
	for i := range want {
		if got.Operations[i] != want[i] {
			t.Errorf("Operations[%d] = %#v, want %#v", i, got.Operations[i], want[i])
		}
	}
}

func TestInspectFileLoadsJSON(t *testing.T) {
	got, err := InspectFile(fixturePath("customer-api.json"))
	if err != nil {
		t.Fatalf("InspectFile() error = %v", err)
	}
	if got.OpenAPIVersion != "3.1.0" || got.Title != "JSON Customer API" || got.APIVersion != "2.0.0" {
		t.Fatalf("Inspection = %#v", got)
	}
	if len(got.Operations) != 1 || got.Operations[0] != (Operation{
		OperationID: "listJSONCustomers",
		Method:      "GET",
		Path:        "/customers",
	}) {
		t.Fatalf("Operations = %#v", got.Operations)
	}
}

func TestInspectFilePreservesMissingOperationIDAndEmptyServer(t *testing.T) {
	got, err := InspectFile(fixturePath("missing-operation-id.yaml"))
	if err != nil {
		t.Fatalf("InspectFile() error = %v", err)
	}
	if got.ServerURL != "" || len(got.Operations) != 1 || got.Operations[0].OperationID != "" {
		t.Fatalf("Inspection = %#v", got)
	}
}

func TestInspectFileRejectsMalformedDocument(t *testing.T) {
	_, err := InspectFile(fixturePath("malformed-openapi.yaml"))
	if err == nil || !strings.Contains(err.Error(), "load document") {
		t.Fatalf("error = %v, want parse/load context", err)
	}
}

func TestInspectFileRejectsInvalidDocument(t *testing.T) {
	_, err := InspectFile(fixturePath("invalid-openapi.yaml"))
	if err == nil || !strings.Contains(err.Error(), "validate document") {
		t.Fatalf("error = %v, want validation context", err)
	}
}

func TestInspectFileRejectsUnsupportedVersion(t *testing.T) {
	_, err := InspectFile(fixturePath("unsupported-version.yaml"))
	if err == nil || !strings.Contains(err.Error(), "unsupported OpenAPI version") {
		t.Fatalf("error = %v, want unsupported version context", err)
	}
}

func TestInspectFileReportsUnreadablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	_, err := InspectFile(path)
	if err == nil || !strings.Contains(err.Error(), "load document") || !strings.Contains(err.Error(), "missing.yaml") {
		t.Fatalf("error = %v, want load context and filename", err)
	}
}

func TestInspectFileRejectsExternalReferences(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "openapi.yaml")
	shared := filepath.Join(dir, "shared.yaml")

	rootDocument := `openapi: 3.0.3
info:
  title: External Ref API
  version: 1.0.0
paths:
  /items:
    get:
      operationId: listItems
      responses:
        "200":
          description: Items
          content:
            application/json:
              schema:
                $ref: "./shared.yaml#/Item"
`
	sharedDocument := `Item:
  type: object
  properties:
    id:
      type: string
`

	if err := os.WriteFile(root, []byte(rootDocument), 0o600); err != nil {
		t.Fatalf("write root fixture: %v", err)
	}
	if err := os.WriteFile(shared, []byte(sharedDocument), 0o600); err != nil {
		t.Fatalf("write shared fixture: %v", err)
	}

	_, err := InspectFile(root)
	if err == nil || !strings.Contains(err.Error(), "disallowed external reference") {
		t.Fatalf("error = %v, want external reference context", err)
	}
}

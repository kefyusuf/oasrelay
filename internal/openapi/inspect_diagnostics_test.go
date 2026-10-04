package openapi

import (
	"reflect"
	"strings"
	"testing"
)

func TestInspectDiagnosticsMatchesSelectorWithoutChangingDiscovery(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Diagnostics API
  version: 1.0.0
servers:
  - url: /relative
paths:
  /z-blocked:
    get:
      operationId: blocked
      parameters:
        - {name: a, in: query, schema: {type: string}}
        - {name: b, in: query, schema: {type: string}}
        - {name: c, in: query, schema: {type: string}}
      responses:
        '200': {description: OK}
  /a-supported/{id}:
    parameters:
      - name: id
        in: path
        required: true
        schema: {type: string, enum: [active, archived]}
      - name: filter
        in: query
        required: true
        schema: {type: string, pattern: '^legacy$'}
    get:
      operationId: supported
      servers:
        - url: https://example.test/api
      parameters:
        - name: filter
          in: query
          schema: {type: string, enum: [current]}
      responses:
        '200': {description: OK}
  /b-no-server:
    get:
      operationId: noServer
      responses:
        '200': {description: OK}
  /c-missing-id:
    get:
      responses:
        '200': {description: OK}
  /d-post-only:
    post:
      operationId: postOnly
      responses:
        '200': {description: OK}
`)
	plain, err := InspectFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := InspectFileWithDiagnostics(path)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Diagnostics != nil {
		t.Fatalf("default diagnostics = %#v", plain.Diagnostics)
	}
	if !reflect.DeepEqual(plain.Operations, got.Operations) || plain.Title != got.Title || plain.ServerURL != got.ServerURL {
		t.Fatalf("discovery changed: plain = %#v, diagnostics = %#v", plain, got)
	}
	wantPaths := []string{"/a-supported/{id}", "/b-no-server", "/c-missing-id", "/z-blocked"}
	if len(got.Operations) != len(wantPaths) || len(got.Diagnostics) != len(wantPaths) {
		t.Fatalf("inspection = %#v", got)
	}
	for index, operation := range got.Operations {
		if operation.Path != wantPaths[index] {
			t.Fatalf("order = %#v", got.Operations)
		}
		_, err := SelectGET(path, operation.OperationID)
		diagnostic, exists := got.Diagnostics[operation.Path]
		if !exists || diagnostic.Selectable != (err == nil) {
			t.Fatalf("%s diagnostic = %#v; selection error = %v", operation.Path, diagnostic, err)
		}
		if err != nil && diagnostic.Reason != err.Error() {
			t.Fatalf("reason = %q; selector = %q", diagnostic.Reason, err.Error())
		}
		if err == nil && diagnostic.Reason != "" {
			t.Fatalf("supported reason = %q", diagnostic.Reason)
		}
	}
	if !got.Diagnostics["/a-supported/{id}"].Selectable || !strings.Contains(got.Diagnostics["/z-blocked"].Reason, "at most two effective parameters") {
		t.Fatalf("diagnostics = %#v", got.Diagnostics)
	}
}

func TestInspectDiagnosticsHandlesNoGETOperations(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info: {title: Empty, version: 1.0.0}
paths: {}
`)
	got, err := InspectFileWithDiagnostics(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Operations) != 0 || got.Diagnostics == nil || len(got.Diagnostics) != 0 {
		t.Fatalf("inspection = %#v", got)
	}
}

func TestInspectDiagnosticsRejectsDocumentErrors(t *testing.T) {
	for _, fixture := range []string{"invalid-openapi.yaml", "malformed-openapi.yaml", "unsupported-version.yaml"} {
		if _, err := InspectFileWithDiagnostics(fixturePath(fixture)); err == nil {
			t.Fatalf("accepted %s", fixture)
		}
	}
	path := writeSelectionSpec(t, `openapi: 3.0.3
info: {title: External, version: 1.0.0}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          $ref: 'https://example.test/responses.yaml#/OK'
`)
	if _, err := InspectFileWithDiagnostics(path); err == nil || !strings.Contains(err.Error(), "disallowed external reference") {
		t.Fatalf("external reference error = %v", err)
	}
}

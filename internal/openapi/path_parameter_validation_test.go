package openapi

import (
	"strings"
	"testing"
)

func TestSelectGETRejectsBlankPathParameterNameAtDocumentValidation(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Path API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /customers/{}:
    get:
      operationId: getCustomer
      parameters:
        - name: ""
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Customer
`)

	_, err := SelectGET(path, "getCustomer")
	if err == nil || !strings.Contains(err.Error(), "validate document") {
		t.Fatalf("error = %v, want validated-document rejection for blank path parameter name", err)
	}
}

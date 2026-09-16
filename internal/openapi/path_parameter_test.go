package openapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestSelectGETAcceptsOneRequiredPrimitivePathParameter(t *testing.T) {
	tests := []struct {
		name       string
		schemaType string
	}{
		{name: "string", schemaType: "string"},
		{name: "integer", schemaType: "integer"},
		{name: "number", schemaType: "number"},
		{name: "boolean", schemaType: "boolean"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.0.3
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
            type: %s
      responses:
        "200":
          description: Customer
`, test.schemaType))

			got, err := SelectGET(path, "getCustomer")
			if err != nil {
				t.Fatalf("SelectGET() error = %v", err)
			}
			if got.QueryParameter != nil {
				t.Fatalf("QueryParameter = %#v, want nil", got.QueryParameter)
			}
			if got.PathParameter == nil {
				t.Fatal("PathParameter = nil")
			}
			if got.PathParameter.Name != "customerId" || got.PathParameter.Type != test.schemaType {
				t.Fatalf("PathParameter = %#v", got.PathParameter)
			}
			if got.Path != "/customers/{customerId}" {
				t.Fatalf("Path = %q", got.Path)
			}
		})
	}
}

func TestSelectGETResolvesInternalPrimitivePathSchemaReference(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Path API
  version: 1.0.0
servers:
  - url: https://example.test
components:
  schemas:
    CustomerID:
      type: string
paths:
  /customers/{customerId}:
    get:
      operationId: getCustomer
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            $ref: "#/components/schemas/CustomerID"
      responses:
        "200":
          description: Customer
`)

	got, err := SelectGET(path, "getCustomer")
	if err != nil {
		t.Fatalf("SelectGET() error = %v", err)
	}
	if got.PathParameter == nil || got.PathParameter.Name != "customerId" || got.PathParameter.Type != "string" {
		t.Fatalf("PathParameter = %#v", got.PathParameter)
	}
}

func TestSelectGETRejectsPathItemLevelPathParameter(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Path API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /customers/{customerId}:
    parameters:
      - name: customerId
        in: path
        required: true
        schema:
          type: string
    get:
      operationId: getCustomer
      responses:
        "200":
          description: Customer
`)

	_, err := SelectGET(path, "getCustomer")
	if err == nil || !strings.Contains(err.Error(), "path-level parameters") {
		t.Fatalf("error = %v, want path-level parameter rejection", err)
	}
}

func TestSelectGETRejectsCustomPathSerialization(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Path API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /customers/{customerId}:
    get:
      operationId: getCustomer
      parameters:
        - name: customerId
          in: path
          required: true
          style: label
          explode: true
          schema:
            type: string
      responses:
        "200":
          description: Customer
`)

	_, err := SelectGET(path, "getCustomer")
	if err == nil || !strings.Contains(err.Error(), "default path serialization") {
		t.Fatalf("error = %v, want default path serialization rejection", err)
	}
}

func TestSelectGETStillRejectsQueryAndPathCombination(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Path API
  version: 1.0.0
servers:
  - url: https://example.test
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
        - name: expand
          in: query
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Customer
`)

	_, err := SelectGET(path, "getCustomer")
	if err == nil || !strings.Contains(err.Error(), "at most one operation-level parameter") {
		t.Fatalf("error = %v, want one-parameter limit", err)
	}
}

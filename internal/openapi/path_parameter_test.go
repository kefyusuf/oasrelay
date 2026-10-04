package openapi

import (
	"fmt"
	"reflect"
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
			if len(got.QueryParameters) != 0 {
				t.Fatalf("QueryParameters = %#v, want none", got.QueryParameters)
			}
			if len(got.PathParameters) != 1 {
				t.Fatalf("PathParameters = %#v, want exactly one", got.PathParameters)
			}
			if got.PathParameters[0].Name != "customerId" || got.PathParameters[0].Type != test.schemaType {
				t.Fatalf("PathParameters = %#v", got.PathParameters)
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
	if len(got.PathParameters) != 1 || got.PathParameters[0].Name != "customerId" || got.PathParameters[0].Type != "string" {
		t.Fatalf("PathParameters = %#v", got.PathParameters)
	}
}

func TestSelectGETAcceptsPathItemLevelPathParameter(t *testing.T) {
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

	got, err := SelectGET(path, "getCustomer")
	if err != nil || len(got.PathParameters) != 1 || !reflect.DeepEqual(got.PathParameters[0], PathParameter{Name: "customerId", Type: "string"}) {
		t.Fatalf("selection = %#v, error = %v", got, err)
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

func TestSelectGETRejectsRepeatedPathPlaceholder(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Path API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /customers/{customerId}/related/{customerId}:
    get:
      operationId: getRelatedCustomer
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Related customer
`)

	_, err := SelectGET(path, "getRelatedCustomer")
	if err == nil || !strings.Contains(err.Error(), "exactly one path placeholder") {
		t.Fatalf("error = %v, want exactly-one placeholder rejection", err)
	}
}

func TestSelectGETRejectsAdditionalPathPlaceholder(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Path API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /customers/{customerId}/related/{otherId}:
    get:
      operationId: getRelatedCustomer
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Related customer
`)

	_, err := SelectGET(path, "getRelatedCustomer")
	if err == nil || !strings.Contains(err.Error(), "additional path placeholders") {
		t.Fatalf("error = %v, want additional-placeholder rejection", err)
	}
}

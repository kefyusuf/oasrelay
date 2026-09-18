package openapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestSelectGETAcceptsOneRequiredPrimitiveQueryParameter(t *testing.T) {
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
  title: Query API
  version: 1.0.0
servers:
  - url: https://example.test/api
paths:
  /customers:
    get:
      operationId: listCustomers
      parameters:
        - name: filter
          in: query
          required: true
          schema:
            type: %s
      responses:
        "200":
          description: Customer collection
`, test.schemaType))

			got, err := SelectGET(path, "listCustomers")
			if err != nil {
				t.Fatalf("SelectGET() error = %v", err)
			}
			if got.Endpoint != "https://example.test/api/customers" {
				t.Fatalf("Endpoint = %q", got.Endpoint)
			}
			if got.QueryParameter == nil {
				t.Fatal("QueryParameter = nil")
			}
			if got.QueryParameter.Name != "filter" || got.QueryParameter.Type != test.schemaType {
				t.Fatalf("QueryParameter = %#v", got.QueryParameter)
			}
		})
	}
}

func TestSelectGETKeepsParameterlessOperationsSupported(t *testing.T) {
	got, err := SelectGET(fixturePath("single-get-api.yaml"), "listCustomers")
	if err != nil {
		t.Fatalf("SelectGET() error = %v", err)
	}
	if got.QueryParameter != nil {
		t.Fatalf("QueryParameter = %#v, want nil", got.QueryParameter)
	}
}

func TestSelectGETResolvesInternalPrimitiveSchemaReference(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Query API
  version: 1.0.0
servers:
  - url: https://example.test
components:
  schemas:
    Limit:
      type: integer
paths:
  /customers:
    get:
      operationId: listCustomers
      parameters:
        - name: limit
          in: query
          required: true
          schema:
            $ref: "#/components/schemas/Limit"
      responses:
        "200":
          description: Customer collection
`)

	got, err := SelectGET(path, "listCustomers")
	if err != nil {
		t.Fatalf("SelectGET() error = %v", err)
	}
	if got.QueryParameter == nil || got.QueryParameter.Name != "limit" || got.QueryParameter.Type != "integer" {
		t.Fatalf("QueryParameter = %#v", got.QueryParameter)
	}
}

func TestSelectGETRejectsUnsupportedParameterShapes(t *testing.T) {
	tests := []struct {
		name       string
		parameters string
		message    string
	}{
		{
			name: "path level parameter",
			parameters: `    parameters:
      - name: limit
        in: query
        required: true
        schema:
          type: integer
    get:
      operationId: listCustomers`,
			message: "path-level parameters",
		},
		{
			name: "optional parameter",
			parameters: `    get:
      operationId: listCustomers
      parameters:
        - name: limit
          in: query
          schema:
            type: integer`,
			message: "must be required",
		},
		{
			name: "header parameter",
			parameters: `    get:
      operationId: listCustomers
      parameters:
        - name: X-Limit
          in: header
          required: true
          schema:
            type: integer`,
			message: "supports query and path parameters only",
		},
		{
			name: "multiple parameters",
			parameters: `    get:
      operationId: listCustomers
      parameters:
        - name: limit
          in: query
          required: true
          schema:
            type: integer
        - name: cursor
          in: query
          required: true
          schema:
            type: string`,
			message: "at most one query parameter",
		},
		{
			name: "array schema",
			parameters: `    get:
      operationId: listCustomers
      parameters:
        - name: ids
          in: query
          required: true
          schema:
            type: array
            items:
              type: string`,
			message: "plain primitive schema",
		},
		{
			name: "enum schema",
			parameters: `    get:
      operationId: listCustomers
      parameters:
        - name: state
          in: query
          required: true
          schema:
            type: string
            enum: [active, disabled]`,
			message: "plain primitive schema",
		},
		{
			name: "nullable schema",
			parameters: `    get:
      operationId: listCustomers
      parameters:
        - name: filter
          in: query
          required: true
          schema:
            type: string
            nullable: true`,
			message: "plain primitive schema",
		},
		{
			name: "custom serialization",
			parameters: `    get:
      operationId: listCustomers
      parameters:
        - name: filter
          in: query
          required: true
          style: spaceDelimited
          explode: false
          schema:
            type: string`,
			message: "default query serialization",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.0.3
info:
  title: Query API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /customers:
%s
      responses:
        "200":
          description: Customer collection
`, test.parameters))

			_, err := SelectGET(path, "listCustomers")
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want substring %q", err, test.message)
			}
		})
	}
}

func TestSelectGETRejectsOpenAPI31ExclusiveNumericConstraints(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
	}{
		{name: "exclusive minimum", constraint: "exclusiveMinimum: 0"},
		{name: "exclusive maximum", constraint: "exclusiveMaximum: 100"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.1.0
info:
  title: Query API
  version: 1.0.0
servers:
  - url: https://example.test
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
            %s
      responses:
        "200":
          description: Customer collection
`, test.constraint))

			_, err := SelectGET(path, "listCustomers")
			if err == nil || !strings.Contains(err.Error(), "plain primitive schema") {
				t.Fatalf("error = %v, want plain primitive schema rejection", err)
			}
		})
	}
}

func TestSelectGETRejectsOpenAPI31ConditionalSchemaConstraints(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
	}{
		{
			name: "if then",
			constraint: `if:
              minimum: 0
            then:
              maximum: 10`,
		},
		{
			name: "else",
			constraint: `else:
              minimum: 0`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.1.0
info:
  title: Query API
  version: 1.0.0
servers:
  - url: https://example.test
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
            %s
      responses:
        "200":
          description: Customer collection
`, test.constraint))

			_, err := SelectGET(path, "listCustomers")
			if err == nil || !strings.Contains(err.Error(), "plain primitive schema") {
				t.Fatalf("error = %v, want plain primitive schema rejection", err)
			}
		})
	}
}

func TestSelectGETRejectsBlankQueryParameterNameDuringDocumentValidation(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Query API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  /customers:
    get:
      operationId: listCustomers
      parameters:
        - name: ""
          in: query
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: Customer collection
`)

	_, err := SelectGET(path, "listCustomers")
	if err == nil || !strings.Contains(err.Error(), "parameter name can't be blank") {
		t.Fatalf("error = %v, want blank parameter name validation error", err)
	}
}

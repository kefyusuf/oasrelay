package openapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
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
			if len(got.QueryParameters) != 1 {
				t.Fatalf("len(QueryParameters) = %d, want 1", len(got.QueryParameters))
			}
			if got.QueryParameters[0].Name != "filter" ||
				got.QueryParameters[0].Type != test.schemaType ||
				got.QueryParameters[0].Optional {
				t.Fatalf("QueryParameters = %#v", got.QueryParameters)
			}
		})
	}
}

func TestSelectGETAcceptsOneOptionalPrimitiveQueryParameter(t *testing.T) {
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
			if len(got.QueryParameters) != 1 {
				t.Fatalf("len(QueryParameters) = %d, want 1", len(got.QueryParameters))
			}
			if got.QueryParameters[0].Name != "filter" ||
				got.QueryParameters[0].Type != test.schemaType ||
				!got.QueryParameters[0].Optional {
				t.Fatalf("QueryParameters = %#v", got.QueryParameters)
			}
		})
	}
}

func TestSelectGETAcceptsTwoPrimitiveQueryParametersInDeclarationOrder(t *testing.T) {
	tests := []struct {
		name           string
		limitRequired  bool
		cursorRequired bool
	}{
		{name: "required plus required", limitRequired: true, cursorRequired: true},
		{name: "required plus optional", limitRequired: true, cursorRequired: false},
		{name: "optional plus required", limitRequired: false, cursorRequired: true},
		{name: "optional plus optional", limitRequired: false, cursorRequired: false},
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
        - name: limit
          in: query
          required: %t
          schema:
            type: integer
        - name: cursor
          in: query
          required: %t
          schema:
            type: string
      responses:
        "200":
          description: Customer collection
`, test.limitRequired, test.cursorRequired))

			got, err := SelectGET(path, "listCustomers")
			if err != nil {
				t.Fatalf("SelectGET() error = %v", err)
			}
			if len(got.QueryParameters) != 2 {
				t.Fatalf("len(QueryParameters) = %d, want 2", len(got.QueryParameters))
			}
			if got.QueryParameters[0].Name != "limit" ||
				got.QueryParameters[0].Type != "integer" ||
				got.QueryParameters[0].Optional == test.limitRequired {
				t.Fatalf("first QueryParameter = %#v", got.QueryParameters[0])
			}
			if got.QueryParameters[1].Name != "cursor" ||
				got.QueryParameters[1].Type != "string" ||
				got.QueryParameters[1].Optional == test.cursorRequired {
				t.Fatalf("second QueryParameter = %#v", got.QueryParameters[1])
			}
		})
	}
}

func TestSupportedOperationParametersRejectsDuplicateQueryNames(t *testing.T) {
	parameters := openapi3.Parameters{
		&openapi3.ParameterRef{Value: &openapi3.Parameter{
			Name:     "filter",
			In:       openapi3.ParameterInQuery,
			Required: true,
			Schema:   &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
		}},
		&openapi3.ParameterRef{Value: &openapi3.Parameter{
			Name:     "filter",
			In:       openapi3.ParameterInQuery,
			Required: false,
			Schema:   &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
		}},
	}

	_, _, err := supportedOperationParameters("listCustomers", "/customers", parameters)
	if err == nil || !strings.Contains(err.Error(), "duplicate query parameter name") {
		t.Fatalf("error = %v, want duplicate-query-name rejection", err)
	}
}

func TestSelectGETTreatsExplicitRequiredFalseQueryAsOptional(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
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
          required: false
          schema:
            type: integer
      responses:
        "200":
          description: Customer collection
`)

	got, err := SelectGET(path, "listCustomers")
	if err != nil {
		t.Fatalf("SelectGET() error = %v", err)
	}
	if len(got.QueryParameters) != 1 ||
		got.QueryParameters[0].Name != "limit" ||
		got.QueryParameters[0].Type != "integer" ||
		!got.QueryParameters[0].Optional {
		t.Fatalf("QueryParameters = %#v", got.QueryParameters)
	}
}

func TestSelectGETKeepsParameterlessOperationsSupported(t *testing.T) {
	got, err := SelectGET(fixturePath("single-get-api.yaml"), "listCustomers")
	if err != nil {
		t.Fatalf("SelectGET() error = %v", err)
	}
	if len(got.QueryParameters) != 0 {
		t.Fatalf("QueryParameters = %#v, want none", got.QueryParameters)
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
	if len(got.QueryParameters) != 1 || got.QueryParameters[0].Name != "limit" || got.QueryParameters[0].Type != "integer" {
		t.Fatalf("QueryParameters = %#v", got.QueryParameters)
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
			name: "three query parameters",
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
            type: string
        - name: sort
          in: query
          required: false
          schema:
            type: string`,
			message: "supports at most two operation-level parameters",
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

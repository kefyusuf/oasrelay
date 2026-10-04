package openapi

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func twoPathSelectionDocument(route, parameters string) string {
	return fmt.Sprintf(`openapi: 3.0.3
info:
  title: Two Path API
  version: 1.0.0
servers:
  - url: https://example.test/api?token=a;b
paths:
  %s:
    get:
      operationId: getOrder
      parameters:
%s
      responses:
        "200":
          description: Order
`, route, parameters)
}

func twoPathParameterYAML(name, schemaType string) string {
	return fmt.Sprintf(`        - name: %s
          in: path
          required: true
          schema:
            type: %s`, name, schemaType)
}

func TestSelectGETAcceptsTwoRequiredPrimitivePathsInDeclarationOrder(t *testing.T) {
	for _, position := range []int{0, 1} {
		for _, schemaType := range []string{"string", "integer", "number", "boolean"} {
			t.Run(fmt.Sprintf("position_%d_%s", position, schemaType), func(t *testing.T) {
				types := []string{"string", "string"}
				types[position] = schemaType
				// Declaration order deliberately differs from route order.
				parameters := twoPathParameterYAML("orderId", types[0]) + "\n" +
					twoPathParameterYAML("customerId", types[1])
				route := "/customers/{customerId}/orders/{orderId}"
				path := writeSelectionSpec(t, twoPathSelectionDocument(route, parameters))
				got, err := SelectGET(path, "getOrder")
				if err != nil {
					t.Fatalf("SelectGET() error = %v", err)
				}
				want := []PathParameter{{Name: "orderId", Type: types[0]}, {Name: "customerId", Type: types[1]}}
				if !reflect.DeepEqual(got.PathParameters, want) {
					t.Fatalf("PathParameters = %#v, want %#v", got.PathParameters, want)
				}
				if len(got.QueryParameters) != 0 || got.Path != route || got.Method != "GET" || got.OperationID != "getOrder" {
					t.Fatalf("selected operation = %#v", got)
				}
				wantEndpoint := "https://example.test/api/customers/%7BcustomerId%7D/orders/%7BorderId%7D?token=a;b"
				if got.Endpoint != wantEndpoint {
					t.Fatalf("Endpoint = %q, want %q", got.Endpoint, wantEndpoint)
				}
			})
		}
	}
}

func TestSelectGETRejectsUnsupportedSecondPathParameter(t *testing.T) {
	first := twoPathParameterYAML("customerId", "string")
	second := twoPathParameterYAML("orderId", "string")
	tests := []struct{ name, parameter, message string }{
		{"optional", strings.Replace(second, "required: true", "required: false", 1), "required"},
		{"custom serialization", strings.Replace(second, "required: true", "required: true\n          style: label", 1), "default path serialization"},
		{"constrained schema", second + "\n            enum: [active]", "plain primitive schema"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeSelectionSpec(t, twoPathSelectionDocument("/customers/{customerId}/orders/{orderId}", first+"\n"+test.parameter))
			_, err := SelectGET(path, "getOrder")
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("SelectGET() error = %v, want %q rejection", err, test.message)
			}
		})
	}
}

func TestSupportedOperationParametersRejectsInvalidTwoPathPlaceholders(t *testing.T) {
	tests := []struct{ name, route, secondName, message string }{
		{"duplicate names", "/customers/{customerId}", "customerId", "duplicate path parameter name"},
		{"repeated placeholder", "/customers/{customerId}/orders/{orderId}/related/{orderId}", "orderId", "exactly one path placeholder"},
		{"missing placeholder", "/customers/{customerId}/orders", "orderId", "exactly one path placeholder"},
		{"undeclared placeholder", "/customers/{customerId}/orders/{orderId}/items/{itemId}", "orderId", "additional path placeholders"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parameters := openapi3.Parameters{}
			for _, name := range []string{"customerId", test.secondName} {
				parameters = append(parameters, &openapi3.ParameterRef{Value: &openapi3.Parameter{
					Name: name, In: openapi3.ParameterInPath, Required: true,
					Schema: &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
				}})
			}
			_, _, err := supportedOperationParameters("getOrder", test.route, parameters)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("supportedOperationParameters() error = %v, want %q rejection", err, test.message)
			}
		})
	}
}

func TestSelectGETRejectsMoreThanTwoParametersWithTwoPaths(t *testing.T) {
	base := twoPathParameterYAML("customerId", "string") + "\n" + twoPathParameterYAML("orderId", "integer")
	tests := []struct{ name, route, extra string }{
		{"two paths plus query", "/customers/{customerId}/orders/{orderId}", `        - name: limit
          in: query
          schema:
            type: integer`},
		{"three paths", "/customers/{customerId}/orders/{orderId}/items/{itemId}", twoPathParameterYAML("itemId", "string")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeSelectionSpec(t, twoPathSelectionDocument(test.route, base+"\n"+test.extra))
			_, err := SelectGET(path, "getOrder")
			if err == nil || !strings.Contains(err.Error(), "at most two operation-level parameters") {
				t.Fatalf("SelectGET() error = %v, want total-parameter-cap rejection", err)
			}
		})
	}
}

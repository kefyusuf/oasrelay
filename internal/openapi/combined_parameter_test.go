package openapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestSelectGETAcceptsOnePathAndOneQueryParameterInEitherOrder(t *testing.T) {
	tests := []struct {
		name       string
		parameters string
	}{
		{
			name: "path then query",
			parameters: `        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: limit
          in: query
          required: true
          schema:
            type: integer`,
		},
		{
			name: "query then path",
			parameters: `        - name: limit
          in: query
          required: true
          schema:
            type: integer
        - name: customerId
          in: path
          required: true
          schema:
            type: string`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.0.3
info:
  title: Combined API
  version: 1.0.0
servers:
  - url: https://example.test/api?token=a;b
paths:
  /customers/{customerId}/orders:
    get:
      operationId: getCustomerOrders
      parameters:
%s
      responses:
        "200":
          description: Customer orders
`, test.parameters))

			got, err := SelectGET(path, "getCustomerOrders")
			if err != nil {
				t.Fatalf("SelectGET() error = %v", err)
			}
			if len(got.PathParameters) != 1 ||
				got.PathParameters[0].Name != "customerId" ||
				got.PathParameters[0].Type != "string" {
				t.Fatalf("PathParameters = %#v", got.PathParameters)
			}
			if len(got.QueryParameters) != 1 ||
				got.QueryParameters[0].Name != "limit" ||
				got.QueryParameters[0].Type != "integer" ||
				got.QueryParameters[0].Optional {
				t.Fatalf("QueryParameters = %#v", got.QueryParameters)
			}
			if got.Endpoint != "https://example.test/api/customers/%7BcustomerId%7D/orders?token=a;b" {
				t.Fatalf("Endpoint = %q", got.Endpoint)
			}
		})
	}
}

func TestSelectGETAcceptsRequiredPathAndOptionalQueryParameter(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Combined API
  version: 1.0.0
servers:
  - url: https://example.test/api
paths:
  /customers/{customerId}/orders:
    get:
      operationId: getCustomerOrders
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: limit
          in: query
          required: false
          schema:
            type: integer
      responses:
        "200":
          description: Customer orders
`)

	got, err := SelectGET(path, "getCustomerOrders")
	if err != nil {
		t.Fatalf("SelectGET() error = %v", err)
	}
	if len(got.PathParameters) != 1 ||
		got.PathParameters[0].Name != "customerId" ||
		got.PathParameters[0].Type != "string" {
		t.Fatalf("PathParameters = %#v", got.PathParameters)
	}
	if len(got.QueryParameters) != 1 ||
		got.QueryParameters[0].Name != "limit" ||
		got.QueryParameters[0].Type != "integer" ||
		!got.QueryParameters[0].Optional {
		t.Fatalf("QueryParameters = %#v", got.QueryParameters)
	}
}

func TestSelectGETRejectsUnsupportedCombinedParameterShapes(t *testing.T) {
	tests := []struct {
		name       string
		route      string
		parameters string
		message    string
	}{
		{
			name:  "three operation parameters",
			route: "/customers/{customerId}",
			parameters: `        - name: customerId
          in: path
          required: true
          schema:
            type: string
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
			message: "supports at most two effective parameters",
		},
		{
			name:  "same MCP argument name across locations",
			route: "/customers/{id}",
			parameters: `        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: id
          in: query
          required: true
          schema:
            type: string`,
			message: "share parameter name",
		},
		{
			name:  "header with path",
			route: "/customers/{customerId}",
			parameters: `        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: X-Limit
          in: header
          required: true
          schema:
            type: integer`,
			message: "supports query and path parameters only",
		},
		{
			name:  "cookie with path",
			route: "/customers/{customerId}",
			parameters: `        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: session
          in: cookie
          required: true
          schema:
            type: string`,
			message: "supports query and path parameters only",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.0.3
info:
  title: Combined API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  %s:
    get:
      operationId: combinedOperation
      parameters:
%s
      responses:
        "200":
          description: Combined operation
`, test.route, test.parameters))

			_, err := SelectGET(path, "combinedOperation")
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want substring %q", err, test.message)
			}
		})
	}
}

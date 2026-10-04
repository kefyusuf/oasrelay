package openapi

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func inheritanceParameter(name, location, schemaType string, required bool) *openapi3.ParameterRef {
	return &openapi3.ParameterRef{Value: &openapi3.Parameter{
		Name: name, In: location, Required: required,
		Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{schemaType}}},
	}}
}

func selectInheritedParameters(route string, inherited, declared openapi3.Parameters) (SelectedOperation, error) {
	return selectGETOperation(&openapi3.T{Servers: openapi3.Servers{{URL: "https://example.test"}}}, route, http.MethodGet,
		&openapi3.PathItem{Parameters: inherited}, &openapi3.Operation{OperationID: "getItems", Parameters: declared})
}

func TestSelectGETMergesInheritedParameters(t *testing.T) {
	query := func(name, typ string, required bool) *openapi3.ParameterRef {
		return inheritanceParameter(name, "query", typ, required)
	}
	path := func(name, typ string) *openapi3.ParameterRef {
		return inheritanceParameter(name, "path", typ, true)
	}
	tests := []struct {
		name, route         string
		inherited, declared openapi3.Parameters
		query               []QueryParameter
		path                []PathParameter
	}{
		{"two inherited paths", "/{first}/{second}", openapi3.Parameters{path("second", "integer"), path("first", "string")}, nil, nil, []PathParameter{{"second", "integer"}, {"first", "string"}}},
		{"mixed sources", "/{id}", openapi3.Parameters{path("id", "string")}, openapi3.Parameters{query("limit", "integer", false)}, []QueryParameter{{"limit", "integer", true}}, []PathParameter{{"id", "string"}}},
		{"inherited queries independent requiredness", "/items", openapi3.Parameters{query("first", "string", true), query("second", "boolean", false)}, nil, []QueryParameter{{"first", "string", false}, {"second", "boolean", true}}, nil},
		{"query override type and requiredness", "/items", openapi3.Parameters{query("first", "string", true)}, openapi3.Parameters{query("first", "integer", false), query("second", "boolean", true)}, []QueryParameter{{"first", "integer", true}, {"second", "boolean", false}}, nil},
		{"query override makes required", "/items", openapi3.Parameters{query("first", "string", false)}, openapi3.Parameters{query("first", "boolean", true)}, []QueryParameter{{"first", "boolean", false}}, nil},
		{"operation-only queries retain order", "/items", nil, openapi3.Parameters{query("second", "boolean", false), query("first", "integer", true)}, []QueryParameter{{"second", "boolean", true}, {"first", "integer", false}}, nil},
		{"reverse overrides retain slots", "/items", openapi3.Parameters{query("first", "string", false), query("second", "string", true)}, openapi3.Parameters{query("second", "boolean", false), query("first", "integer", true)}, []QueryParameter{{"first", "integer", false}, {"second", "boolean", true}}, nil},
		{"path override type", "/{id}", openapi3.Parameters{path("id", "string")}, openapi3.Parameters{path("id", "integer")}, nil, []PathParameter{{"id", "integer"}}},
		{"reverse path overrides retain slots", "/{first}/{second}", openapi3.Parameters{path("second", "string"), path("first", "string")}, openapi3.Parameters{path("first", "boolean"), path("second", "integer")}, nil, []PathParameter{{"second", "integer"}, {"first", "boolean"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inheritedBefore := append(openapi3.Parameters(nil), test.inherited...)
			declaredBefore := append(openapi3.Parameters(nil), test.declared...)
			got, err := selectInheritedParameters(test.route, test.inherited, test.declared)
			if err != nil {
				t.Fatalf("selection error = %v", err)
			}
			if !reflect.DeepEqual(got.QueryParameters, test.query) && !(len(got.QueryParameters) == 0 && len(test.query) == 0) {
				t.Fatalf("query = %#v, want %#v", got.QueryParameters, test.query)
			}
			if !reflect.DeepEqual(got.PathParameters, test.path) && !(len(got.PathParameters) == 0 && len(test.path) == 0) {
				t.Fatalf("path = %#v, want %#v", got.PathParameters, test.path)
			}
			if !reflect.DeepEqual(test.inherited, inheritedBefore) || !reflect.DeepEqual(test.declared, declaredBefore) {
				t.Fatal("source parameters were mutated")
			}
		})
	}
}

func TestSelectGETRejectsInvalidInheritedParameters(t *testing.T) {
	q := inheritanceParameter("q", "query", "string", true)
	id := inheritanceParameter("id", "path", "string", true)
	custom := inheritanceParameter("q", "query", "string", true)
	custom.Value.Style = "spaceDelimited"
	constrained := inheritanceParameter("q", "query", "string", true)
	constrained.Value.Schema.Value.Enum = []any{"active"}
	optionalPath := inheritanceParameter("id", "path", "string", false)
	customPath := inheritanceParameter("id", "path", "string", true)
	customPath.Value.Style = "label"
	tests := []struct {
		name, route         string
		inherited, declared openapi3.Parameters
		message             string
	}{
		{"same name different location", "/{id}", openapi3.Parameters{id}, openapi3.Parameters{inheritanceParameter("id", "query", "string", true)}, "MCP input names must be unique"},
		{"three effective mixed sources", "/{id}", openapi3.Parameters{id, q}, openapi3.Parameters{inheritanceParameter("extra", "query", "boolean", true)}, "at most two"},
		{"three inherited", "/items", openapi3.Parameters{q, inheritanceParameter("second", "query", "string", false), inheritanceParameter("third", "query", "string", true)}, nil, "at most two"},
		{"duplicate inherited overridden", "/items", openapi3.Parameters{q, q}, openapi3.Parameters{q}, "duplicate"},
		{"duplicate operation", "/items", nil, openapi3.Parameters{q, q}, "duplicate"},
		{"unresolved inherited", "/items", openapi3.Parameters{nil}, nil, "unresolved"},
		{"unresolved operation", "/items", openapi3.Parameters{q}, openapi3.Parameters{{Ref: "#/components/parameters/Missing"}}, "unresolved"},
		{"header", "/items", openapi3.Parameters{inheritanceParameter("X-Token", "header", "string", true)}, nil, "query and path parameters only"},
		{"constrained schema", "/items", openapi3.Parameters{constrained}, nil, "plain primitive schema"},
		{"custom serialization", "/items", openapi3.Parameters{custom}, nil, "default query serialization"},
		{"custom path serialization", "/{id}", openapi3.Parameters{customPath}, nil, "default path serialization"},
		{"undeclared placeholder", "/{id}/{other}", openapi3.Parameters{id}, nil, "additional path placeholders"},
		{"repeated placeholder", "/{id}/{id}", openapi3.Parameters{id}, nil, "exactly one path placeholder"},
		{"path requiredness override", "/{id}", openapi3.Parameters{id}, openapi3.Parameters{optionalPath}, "must be required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := selectInheritedParameters(test.route, test.inherited, test.declared)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestSelectGETResolvesInheritedInternalReferences(t *testing.T) {
	path := writeSelectionSpec(t, `openapi: 3.0.3
info:
  title: Inheritance API
  version: 1.0.0
servers:
  - url: https://example.test
components:
  schemas:
    ID:
      type: integer
  parameters:
    ID:
      name: id
      in: path
      required: true
      schema:
        $ref: '#/components/schemas/ID'
paths:
  /items/{id}:
    parameters:
      - $ref: '#/components/parameters/ID'
    get:
      operationId: getItems
      responses:
        '200':
          description: Items
`)
	got, err := SelectGET(path, "getItems")
	if err != nil || !reflect.DeepEqual(got.PathParameters, []PathParameter{{"id", "integer"}}) {
		t.Fatalf("selection = %#v, error = %v", got, err)
	}
	if _, err := SelectParameterlessGET(path, "getItems"); err == nil || !strings.Contains(err.Error(), "parameterless operations only") {
		t.Fatalf("parameterless selection error = %v", err)
	}
}

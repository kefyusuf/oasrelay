package openapi

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func enumParameter(name, location string, required bool, values []any) *openapi3.ParameterRef {
	p := inheritanceParameter(name, location, "string", required)
	p.Value.Schema.Value.Enum = values
	return p
}

func TestSelectGETAcceptsStringEnumsAndInternalReferences(t *testing.T) {
	for _, required := range []bool{false, true} {
		t.Run(fmt.Sprintf("query_required_%t", required), func(t *testing.T) {
			path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.0.3
info:
  title: String Enum API
  version: 1.0.0
servers:
  - url: https://example.test/api
components:
  schemas:
    State:
      type: string
      enum: ["", "Active", " active ", "a/b"]
paths:
  /items/{state}:
    get:
      operationId: getItems
      parameters:
        - name: filter
          in: query
          required: %t
          schema:
            $ref: '#/components/schemas/State'
        - name: state
          in: path
          required: true
          schema:
            $ref: '#/components/schemas/State'
      responses:
        '200':
          description: Items
`, required))
			got, err := SelectGET(path, "getItems")
			if err != nil {
				t.Fatalf("SelectGET() error = %v", err)
			}
			values := []string{"", "Active", " active ", "a/b"}
			wantQuery := []QueryParameter{{Name: "filter", Type: "string", Optional: !required, Enum: values}}
			wantPath := []PathParameter{{Name: "state", Type: "string", Enum: values}}
			if !reflect.DeepEqual(got.QueryParameters, wantQuery) || !reflect.DeepEqual(got.PathParameters, wantPath) {
				t.Fatalf("query = %#v; path = %#v", got.QueryParameters, got.PathParameters)
			}
			if got.Endpoint != "https://example.test/api/items/%7Bstate%7D" {
				t.Fatalf("Endpoint = %q", got.Endpoint)
			}
		})
	}
}

func TestSelectGETStringEnumInheritanceAndWholeParameterOverride(t *testing.T) {
	for _, location := range []string{"query", "path"} {
		for _, mode := range []string{"inherit", "replace", "remove"} {
			t.Run(location+"_"+mode, func(t *testing.T) {
				route := "/items"
				if location == "path" {
					route += "/{state}/{other}"
				}
				inherited := openapi3.Parameters{
					enumParameter("state", location, true, []any{"old", " original "}),
					enumParameter("other", location, true, []any{"second", "first"}),
				}
				var declared openapi3.Parameters
				want := []string{"old", " original "}
				if mode == "replace" {
					declared = openapi3.Parameters{enumParameter("state", location, true, []any{"new"})}
					want = []string{"new"}
				} else if mode == "remove" {
					declared = openapi3.Parameters{enumParameter("state", location, true, nil)}
					want = nil
				}
				got, err := selectInheritedParameters(route, inherited, declared)
				if err != nil {
					t.Fatalf("selection error = %v", err)
				}
				if location == "query" {
					expected := []QueryParameter{{Name: "state", Type: "string", Enum: want}, {Name: "other", Type: "string", Enum: []string{"second", "first"}}}
					if !reflect.DeepEqual(got.QueryParameters, expected) || len(got.PathParameters) != 0 {
						t.Fatalf("selected operation = %#v, want queries %#v", got, expected)
					}
				} else {
					expected := []PathParameter{{Name: "state", Type: "string", Enum: want}, {Name: "other", Type: "string", Enum: []string{"second", "first"}}}
					if !reflect.DeepEqual(got.PathParameters, expected) || len(got.QueryParameters) != 0 {
						t.Fatalf("selected operation = %#v, want paths %#v", got, expected)
					}
				}
				if !reflect.DeepEqual(inherited[0].Value.Schema.Value.Enum, []any{"old", " original "}) {
					t.Fatal("inherited schema enum was mutated")
				}
			})
		}
	}
}

func TestSupportedParametersPreserveAbsentEnum(t *testing.T) {
	for _, location := range []string{"query", "path"} {
		p := enumParameter("state", location, true, nil)
		route := "/items"
		if location == "path" {
			route += "/{state}"
		}
		queries, paths, err := supportedOperationParameters("getItems", route, openapi3.Parameters{p})
		if err != nil {
			t.Fatalf("%s selection error = %v", location, err)
		}
		if location == "query" && (len(queries) != 1 || queries[0].Enum != nil) || location == "path" && (len(paths) != 1 || paths[0].Enum != nil) {
			t.Fatalf("%s absent enum: query = %#v; path = %#v", location, queries, paths)
		}
	}
}

func TestSupportedParametersRejectInvalidStringEnums(t *testing.T) {
	tests := []struct {
		name, schemaType string
		values           []any
		constraint       func(*openapi3.Schema)
	}{
		{"empty", "string", []any{}, nil},
		{"duplicate", "string", []any{"active", "active"}, nil},
		{"mixed", "string", []any{"active", 1}, nil},
		{"null member", "string", []any{"active", nil}, nil},
		{"integer", "integer", []any{1, 2}, nil},
		{"number", "number", []any{1.5}, nil},
		{"boolean", "boolean", []any{true, false}, nil},
		{"default", "string", []any{"active"}, func(s *openapi3.Schema) { s.Default = "active" }},
		{"format", "string", []any{"active"}, func(s *openapi3.Schema) { s.Format = "uuid" }},
		{"pattern", "string", []any{"active"}, func(s *openapi3.Schema) { s.Pattern = "^active$" }},
	}
	for _, location := range []string{"query", "path"} {
		for _, test := range tests {
			t.Run(location+"_"+test.name, func(t *testing.T) {
				p := inheritanceParameter("state", location, test.schemaType, true)
				p.Value.Schema.Value.Enum = test.values
				if test.constraint != nil {
					test.constraint(p.Value.Schema.Value)
				}
				route := "/items"
				if location == "path" {
					route += "/{state}"
				}
				if _, _, err := supportedOperationParameters("getItems", route, openapi3.Parameters{p}); err == nil {
					t.Fatal("invalid enum schema was accepted")
				}
			})
		}
	}
}

func TestSelectGETStringEnumsKeepTotalParameterCap(t *testing.T) {
	parameters := openapi3.Parameters{
		enumParameter("first", "query", true, []any{"a"}),
		enumParameter("second", "query", false, []any{"b"}),
		enumParameter("third", "query", true, []any{"c"}),
	}
	if _, err := selectInheritedParameters("/items", parameters[:2], parameters[2:]); err == nil || !strings.Contains(err.Error(), "at most two") {
		t.Fatalf("selection error = %v, want total parameter cap rejection", err)
	}
}

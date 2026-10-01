package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExposesRequiredPrimitiveQueryParameterSchema(t *testing.T) {
	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    "http://example.test/customers",
		QueryParameters: []oasopenapi.QueryParameter{{
			Name: "limit",
			Type: "integer",
		}},
	}, http.DefaultClient)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(listed.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(listed.Tools))
	}

	encoded, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}

	if schema["type"] != "object" {
		t.Fatalf("input schema = %s, want object type", encoded)
	}
	additionalProperties, ok := schema["additionalProperties"].(bool)
	if !ok || additionalProperties {
		t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
	}
	required, ok := schema["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "limit" {
		t.Fatalf("required = %#v, want [limit]", schema["required"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", schema["properties"])
	}
	limit, ok := properties["limit"].(map[string]any)
	if !ok || limit["type"] != "integer" {
		t.Fatalf("limit schema = %#v, want integer", properties["limit"])
	}
}

func TestServerExposesOptionalPrimitiveQueryInputSchema(t *testing.T) {
	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    "http://example.test/customers",
		QueryParameters: []oasopenapi.QueryParameter{{
			Name:     "limit",
			Type:     "integer",
			Optional: true,
		}},
	}, http.DefaultClient)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(listed.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(listed.Tools))
	}

	encoded, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", schema["properties"])
	}
	limit, ok := properties["limit"].(map[string]any)
	if !ok || limit["type"] != "integer" {
		t.Fatalf("limit schema = %#v", properties["limit"])
	}
	if _, exists := schema["required"]; exists {
		t.Fatalf("required = %#v, want field omitted", schema["required"])
	}
	if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
		t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
	}
}

func TestServerBindsPrimitiveQueryParameter(t *testing.T) {
	tests := []struct {
		name       string
		paramType  string
		argument   any
		wantValue  string
		wantEscape string
	}{
		{
			name:       "string",
			paramType:  "string",
			argument:   "a b&c",
			wantValue:  "a b&c",
			wantEscape: "filter=a+b%26c",
		},
		{name: "integer", paramType: "integer", argument: 25, wantValue: "25", wantEscape: "filter=25"},
		{
			name:       "integer decimal token",
			paramType:  "integer",
			argument:   json.Number("25.0"),
			wantValue:  "25",
			wantEscape: "filter=25",
		},
		{
			name:       "integer exponent token",
			paramType:  "integer",
			argument:   json.Number("25e0"),
			wantValue:  "25",
			wantEscape: "filter=25",
		},
		{name: "number", paramType: "number", argument: 1.25, wantValue: "1.25", wantEscape: "filter=1.25"},
		{name: "boolean", paramType: "boolean", argument: true, wantValue: "true", wantEscape: "filter=true"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			requestURL := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				requestURL <- request.URL.String()
				if got := request.URL.Query().Get("filter"); got != test.wantValue {
					t.Errorf("query filter = %q, want %q", got, test.wantValue)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(oasopenapi.SelectedOperation{
				OperationID: "listCustomers",
				Method:      http.MethodGet,
				Path:        "/customers",
				Endpoint:    upstream.URL + "/customers",
				QueryParameters: []oasopenapi.QueryParameter{{
					Name: "filter",
					Type: test.paramType,
				}},
			}, upstream.Client())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "listCustomers",
				Arguments: map[string]any{"filter": test.argument},
			})
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if result.IsError {
				t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
			}
			if calls.Load() != 1 {
				t.Fatalf("upstream calls = %d, want 1", calls.Load())
			}

			gotURL := <-requestURL
			if !strings.Contains(gotURL, test.wantEscape) {
				t.Fatalf("request URL = %q, want encoded query %q", gotURL, test.wantEscape)
			}
		})
	}
}

func TestServerBindsOptionalQueryWhenOmittedOrSupplied(t *testing.T) {
	var calls atomic.Int32
	requests := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		requests <- request.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    upstream.URL + "/customers?token=a;b&mode=raw%2Fvalue",
		QueryParameters: []oasopenapi.QueryParameter{{
			Name:     "limit",
			Type:     "integer",
			Optional: true,
		}},
	}, upstream.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "listCustomers",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool() omitted optional query error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() omitted optional query IsError = true; content = %#v", result.Content)
	}
	if got := <-requests; got != "/customers?token=a;b&mode=raw%2Fvalue" {
		t.Fatalf("omitted optional query RequestURI = %q", got)
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "listCustomers",
		Arguments: map[string]any{
			"limit": json.Number("25e0"),
		},
	})
	if err != nil {
		t.Fatalf("CallTool() supplied optional query error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() supplied optional query IsError = true; content = %#v", result.Content)
	}
	if got := <-requests; got != "/customers?token=a;b&mode=raw%2Fvalue&limit=25" {
		t.Fatalf("supplied optional query RequestURI = %q", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls.Load())
	}
}

func TestServerRejectsInvalidOptionalQueryArgumentsBeforeUpstream(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "explicit null", arguments: map[string]any{"limit": nil}},
		{name: "wrong primitive type", arguments: map[string]any{"limit": "25"}},
		{name: "unknown field", arguments: map[string]any{"cursor": "next"}},
		{name: "optional plus unknown field", arguments: map[string]any{"limit": 25, "cursor": "next"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(oasopenapi.SelectedOperation{
				OperationID: "listCustomers",
				Method:      http.MethodGet,
				Path:        "/customers",
				Endpoint:    upstream.URL + "/customers",
				QueryParameters: []oasopenapi.QueryParameter{{
					Name:     "limit",
					Type:     "integer",
					Optional: true,
				}},
			}, upstream.Client())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "listCustomers",
				Arguments: test.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() protocol error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("CallTool() IsError = false, want true; result = %#v", result)
			}
			if calls.Load() != 0 {
				t.Fatalf("upstream calls = %d, want 0", calls.Load())
			}
		})
	}
}

func TestServerRejectsInvalidQueryArgumentsBeforeUpstream(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "missing required parameter", arguments: map[string]any{}},
		{name: "wrong primitive type", arguments: map[string]any{"limit": "25"}},
		{name: "unknown field", arguments: map[string]any{"limit": 25, "cursor": "next"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(oasopenapi.SelectedOperation{
				OperationID: "listCustomers",
				Method:      http.MethodGet,
				Path:        "/customers",
				Endpoint:    upstream.URL + "/customers",
				QueryParameters: []oasopenapi.QueryParameter{{
					Name: "limit",
					Type: "integer",
				}},
			}, upstream.Client())
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "listCustomers",
				Arguments: test.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() protocol error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("CallTool() IsError = false, want true; result = %#v", result)
			}
			if calls.Load() != 0 {
				t.Fatalf("upstream calls = %d, want 0", calls.Load())
			}
		})
	}
}

func twoQueryOperation(endpoint string, firstOptional, secondOptional bool) oasopenapi.SelectedOperation {
	return oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    endpoint,
		QueryParameters: []oasopenapi.QueryParameter{
			{Name: "limit", Type: "integer", Optional: firstOptional},
			{Name: "cursor", Type: "string", Optional: secondOptional},
		},
	}
}

func TestServerExposesTwoQueryInputSchemaRequiredness(t *testing.T) {
	tests := []struct {
		name           string
		firstOptional  bool
		secondOptional bool
		wantRequired   []any
	}{
		{name: "required plus required", wantRequired: []any{"limit", "cursor"}},
		{name: "required plus optional", secondOptional: true, wantRequired: []any{"limit"}},
		{name: "optional plus required", firstOptional: true, wantRequired: []any{"cursor"}},
		{name: "optional plus optional", firstOptional: true, secondOptional: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, err := New(
				twoQueryOperation(
					"http://example.test/customers",
					test.firstOptional,
					test.secondOptional,
				),
				http.DefaultClient,
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			listed, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatalf("ListTools() error = %v", err)
			}
			if len(listed.Tools) != 1 {
				t.Fatalf("len(Tools) = %d, want 1", len(listed.Tools))
			}

			encoded, err := json.Marshal(listed.Tools[0].InputSchema)
			if err != nil {
				t.Fatalf("marshal input schema: %v", err)
			}
			var schema map[string]any
			if err := json.Unmarshal(encoded, &schema); err != nil {
				t.Fatalf("unmarshal input schema: %v", err)
			}

			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("properties = %#v", schema["properties"])
			}
			limit, ok := properties["limit"].(map[string]any)
			if !ok || limit["type"] != "integer" {
				t.Fatalf("limit schema = %#v", properties["limit"])
			}
			cursor, ok := properties["cursor"].(map[string]any)
			if !ok || cursor["type"] != "string" {
				t.Fatalf("cursor schema = %#v", properties["cursor"])
			}
			if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
				t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
			}

			if len(test.wantRequired) == 0 {
				if _, exists := schema["required"]; exists {
					t.Fatalf("required = %#v, want field omitted", schema["required"])
				}
				return
			}
			required, ok := schema["required"].([]any)
			if !ok || len(required) != len(test.wantRequired) {
				t.Fatalf("required = %#v, want %#v", schema["required"], test.wantRequired)
			}
			for index := range test.wantRequired {
				if required[index] != test.wantRequired[index] {
					t.Fatalf("required = %#v, want %#v", required, test.wantRequired)
				}
			}
		})
	}
}

func TestServerBindsTwoQueryParametersInDeclarationOrder(t *testing.T) {
	requests := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests <- request.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	server, err := New(
		twoQueryOperation(
			upstream.URL+"/customers?token=a;b&mode=raw%2Fvalue",
			false,
			true,
		),
		upstream.Client(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "listCustomers",
		Arguments: map[string]any{
			"limit": 25,
		},
	})
	if err != nil {
		t.Fatalf("CallTool() required-only error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() required-only IsError = true; content = %#v", result.Content)
	}
	if got := <-requests; got != "/customers?token=a;b&mode=raw%2Fvalue&limit=25" {
		t.Fatalf("required-only RequestURI = %q", got)
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "listCustomers",
		Arguments: map[string]any{
			"limit":  25,
			"cursor": "next page",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() both queries error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() both queries IsError = true; content = %#v", result.Content)
	}
	if got := <-requests; got != "/customers?token=a;b&mode=raw%2Fvalue&limit=25&cursor=next+page" {
		t.Fatalf("both queries RequestURI = %q", got)
	}
}

func TestServerCanonicalizesIntegerInSecondQueryPosition(t *testing.T) {
	requests := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests <- request.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	operation := oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    upstream.URL + "/customers",
		QueryParameters: []oasopenapi.QueryParameter{
			{Name: "filter", Type: "string"},
			{Name: "limit", Type: "integer"},
		},
	}
	server, err := New(operation, upstream.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "listCustomers",
		Arguments: map[string]any{
			"filter": "active",
			"limit":  json.Number("25e0"),
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
	}
	if got := <-requests; got != "/customers?filter=active&limit=25" {
		t.Fatalf("RequestURI = %q", got)
	}
}

func TestServerBindsTwoOptionalQueriesWhenOmittedOrSupplied(t *testing.T) {
	requests := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests <- request.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	operation := oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    upstream.URL + "/customers?token=a;b&mode=raw%2Fvalue",
		QueryParameters: []oasopenapi.QueryParameter{
			{Name: "limit", Type: "integer", Optional: true},
			{Name: "filter", Type: "string", Optional: true},
		},
	}
	server, err := New(operation, upstream.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	tests := []struct {
		name      string
		arguments map[string]any
		wantURI   string
	}{
		{
			name:      "both omitted",
			arguments: map[string]any{},
			wantURI:   "/customers?token=a;b&mode=raw%2Fvalue",
		},
		{
			name:      "only first supplied",
			arguments: map[string]any{"limit": 25},
			wantURI:   "/customers?token=a;b&mode=raw%2Fvalue&limit=25",
		},
		{
			name:      "explicit empty second value",
			arguments: map[string]any{"filter": ""},
			wantURI:   "/customers?token=a;b&mode=raw%2Fvalue&filter=",
		},
		{
			name:      "both supplied",
			arguments: map[string]any{"limit": 25, "filter": "active"},
			wantURI:   "/customers?token=a;b&mode=raw%2Fvalue&limit=25&filter=active",
		},
	}

	for _, test := range tests {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "listCustomers",
			Arguments: test.arguments,
		})
		if err != nil {
			t.Fatalf("%s CallTool() error = %v", test.name, err)
		}
		if result.IsError {
			t.Fatalf("%s CallTool() IsError = true; content = %#v", test.name, result.Content)
		}
		if got := <-requests; got != test.wantURI {
			t.Fatalf("%s RequestURI = %q, want %q", test.name, got, test.wantURI)
		}
	}
}

func TestServerBindsOptionalFirstAndRequiredSecondQuery(t *testing.T) {
	requests := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests <- request.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	operation := twoQueryOperation(upstream.URL+"/customers", true, false)
	server, err := New(operation, upstream.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "listCustomers",
		Arguments: map[string]any{
			"cursor": "next",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
	}
	if got := <-requests; got != "/customers?cursor=next" {
		t.Fatalf("RequestURI = %q", got)
	}
}

func TestServerRejectsInvalidTwoQueryArgumentsBeforeUpstream(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "missing first required", arguments: map[string]any{"cursor": "next"}},
		{name: "missing second required", arguments: map[string]any{"limit": 25}},
		{name: "null first", arguments: map[string]any{"limit": nil, "cursor": "next"}},
		{name: "null second", arguments: map[string]any{"limit": 25, "cursor": nil}},
		{name: "wrong first type", arguments: map[string]any{"limit": "25", "cursor": "next"}},
		{name: "wrong second type", arguments: map[string]any{"limit": 25, "cursor": 10}},
		{name: "unknown field", arguments: map[string]any{"limit": 25, "cursor": "next", "extra": true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()

			server, err := New(
				twoQueryOperation(upstream.URL+"/customers", false, false),
				upstream.Client(),
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			session, cleanup := connectClient(t, server)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "listCustomers",
				Arguments: test.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool() protocol error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("CallTool() IsError = false, want true")
			}
			if calls.Load() != 0 {
				t.Fatalf("upstream calls = %d, want 0", calls.Load())
			}
		})
	}
}

func TestServerRejectsMalformedQueryParameterModels(t *testing.T) {
	tests := []struct {
		name      string
		operation oasopenapi.SelectedOperation
		message   string
	}{
		{
			name: "three queries",
			operation: oasopenapi.SelectedOperation{
				OperationID: "listCustomers",
				Method:      http.MethodGet,
				Path:        "/customers",
				Endpoint:    "http://example.test/customers",
				QueryParameters: []oasopenapi.QueryParameter{
					{Name: "limit", Type: "integer"},
					{Name: "cursor", Type: "string"},
					{Name: "sort", Type: "string"},
				},
			},
			message: "at most two query parameters",
		},
		{
			name: "duplicate query names",
			operation: oasopenapi.SelectedOperation{
				OperationID: "listCustomers",
				Method:      http.MethodGet,
				Path:        "/customers",
				Endpoint:    "http://example.test/customers",
				QueryParameters: []oasopenapi.QueryParameter{
					{Name: "filter", Type: "string"},
					{Name: "filter", Type: "string"},
				},
			},
			message: "duplicate query parameter name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.operation, http.DefaultClient)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("New() error = %v, want substring %q", err, test.message)
			}
		})
	}
}


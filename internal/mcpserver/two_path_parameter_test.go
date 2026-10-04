package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func twoPathOperation(endpoint string) oasopenapi.SelectedOperation {
	return oasopenapi.SelectedOperation{
		OperationID: "getCustomerOrder", Method: http.MethodGet,
		Path: "/customers/{customerId}/orders/{orderId}", Endpoint: endpoint,
		PathParameters: []oasopenapi.PathParameter{
			{Name: "customerId", Type: "string"}, {Name: "orderId", Type: "string"},
		},
	}
}

func TestServerExposesTwoRequiredPathProperties(t *testing.T) {
	server, err := New(twoPathOperation("http://example.test/customers/%7BcustomerId%7D/orders/%7BorderId%7D"), http.DefaultClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	session, cleanup := connectClient(t, server)
	defer cleanup()
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) != 1 {
		t.Fatalf("ListTools = %#v, %v; want one tool", listed, err)
	}
	encoded, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema["required"], []any{"customerId", "orderId"}) {
		t.Fatalf("required = %#v", schema["required"])
	}
	wantProperties := map[string]any{
		"customerId": map[string]any{"type": "string"}, "orderId": map[string]any{"type": "string"},
	}
	if !reflect.DeepEqual(schema["properties"], wantProperties) || schema["additionalProperties"] != false {
		t.Fatalf("schema = %#v, want two closed string properties", schema)
	}
}

func TestServerBindsTwoPathsWithoutReinterpretingCallerData(t *testing.T) {
	for _, tc := range []struct {
		name, first, second, secondType, want string
		reversed                              bool
	}{
		{"plain", "cus_123", "42", "string", "/api/customers/cus_123/orders/42", false},
		{"reversed declaration", "cus_123", "42", "string", "/api/customers/cus_123/orders/42", true},
		{"placeholder in first", "{orderId}", "42", "string", "/api/customers/%7BorderId%7D/orders/42", false},
		{"placeholder in second", "cus_123", "{customerId}", "string", "/api/customers/cus_123/orders/%7BcustomerId%7D", true},
		{"slashes", "a/b", "x/y", "string", "/api/customers/a%2Fb/orders/x%2Fy", false},
		{"dot segments", ".", "..", "string", "/api/customers/%2E/orders/%2E%2E", false},
		{"reversed dot segments", "..", ".", "string", "/api/customers/%2E%2E/orders/%2E", false},
		{"percent and unicode", "%2F", "ç", "string", "/api/customers/%252F/orders/%C3%A7", false},
		{"second integer", "cus_123", "25e0", "integer", "/api/customers/cus_123/orders/25", false},
		{"second number", "cus_123", "1.25", "number", "/api/customers/cus_123/orders/1.25", false},
		{"second boolean", "cus_123", "true", "boolean", "/api/customers/cus_123/orders/true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.RequestURI
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()
			operation := twoPathOperation(upstream.URL + "/api/customers/%7BcustomerId%7D/orders/%7BorderId%7D?token=a;b&mode=raw%2Fvalue")
			operation.PathParameters[1].Type = tc.secondType
			if tc.reversed {
				operation.PathParameters[0], operation.PathParameters[1] = operation.PathParameters[1], operation.PathParameters[0]
			}
			server, err := New(operation, upstream.Client())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			session, cleanup := connectClient(t, server)
			defer cleanup()
			var second any = tc.second
			if tc.secondType != "string" {
				second = json.RawMessage(tc.second)
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: operation.OperationID, Arguments: map[string]any{"customerId": tc.first, "orderId": second},
			})
			if err != nil || result.IsError {
				t.Fatalf("CallTool = %#v, %v", result, err)
			}
			select {
			case got := <-requests:
				if want := tc.want + "?token=a;b&mode=raw%2Fvalue"; got != want {
					t.Fatalf("RequestURI = %q, want %q", got, want)
				}
			default:
				t.Fatal("upstream request missing")
			}
		})
	}
}

func TestServerRejectsInvalidTwoPathInputBeforeUpstream(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"missing first", map[string]any{"orderId": "42"}},
		{"missing second", map[string]any{"customerId": "cus_123"}},
		{"empty input", map[string]any{}},
		{"null first", map[string]any{"customerId": nil, "orderId": "42"}},
		{"null second", map[string]any{"customerId": "cus_123", "orderId": nil}},
		{"wrong first type", map[string]any{"customerId": true, "orderId": "42"}},
		{"wrong second type", map[string]any{"customerId": "cus_123", "orderId": 42}},
		{"unknown extra", map[string]any{"customerId": "cus_123", "orderId": "42", "extra": "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			defer upstream.Close()
			operation := twoPathOperation(upstream.URL + "/customers/%7BcustomerId%7D/orders/%7BorderId%7D")
			server, err := New(operation, upstream.Client())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			session, cleanup := connectClient(t, server)
			defer cleanup()
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: operation.OperationID, Arguments: tc.args})
			if err == nil && (result == nil || !result.IsError) {
				t.Fatalf("CallTool = %#v, want error", result)
			}
			if calls.Load() != 0 {
				t.Fatalf("upstream calls = %d, want zero", calls.Load())
			}
		})
	}
}

func TestServerRejectsMalformedTwoPathModels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*oasopenapi.SelectedOperation)
	}{
		{"duplicate names", func(o *oasopenapi.SelectedOperation) { o.PathParameters[1].Name = "customerId" }},
		{"three paths", func(o *oasopenapi.SelectedOperation) {
			o.PathParameters = append(o.PathParameters, oasopenapi.PathParameter{Name: "third", Type: "string"})
		}},
		{"two paths plus query", func(o *oasopenapi.SelectedOperation) {
			o.QueryParameters = []oasopenapi.QueryParameter{{Name: "limit", Type: "integer"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation := twoPathOperation("http://example.test/customers/%7BcustomerId%7D/orders/%7BorderId%7D")
			tc.modify(&operation)
			if _, err := New(operation, http.DefaultClient); err == nil {
				t.Fatal("New accepted malformed path model")
			}
		})
	}
}

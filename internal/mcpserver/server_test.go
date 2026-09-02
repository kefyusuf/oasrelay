package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExposesAndCallsOneGETTool(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/api/customers" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer upstream.Close()

	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Summary:     "List customers",
		Endpoint:    upstream.URL + "/api/customers",
	}, upstream.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	session, cleanup := connectClient(t, server)
	defer cleanup()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(listed.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(listed.Tools))
	}

	tool := listed.Tools[0]
	if tool.Name != "listCustomers" || tool.Description != "List customers" {
		t.Fatalf("Tool = %#v", tool)
	}
	assertObjectSchema(t, tool.InputSchema)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "listCustomers",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
	}

	got := decodeToolOutput(t, result.StructuredContent)
	want := ToolOutput{
		Status:      http.StatusOK,
		ContentType: "application/json",
		Body:        `{"items":[]}`,
	}
	if got != want || calls.Load() != 1 {
		t.Fatalf("ToolOutput = %#v, calls = %d, want %#v and 1 call", got, calls.Load(), want)
	}
}

func TestServerMarksNon2xxAsToolErrorAndPreservesOutput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"missing"}`)
	}))
	defer upstream.Close()

	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "getMissing",
		Method:      http.MethodGet,
		Path:        "/missing",
		Endpoint:    upstream.URL + "/missing",
	}, upstream.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, cleanup := connectClient(t, server)
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "getMissing",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("CallTool() IsError = false, want true")
	}

	got := decodeToolOutput(t, result.StructuredContent)
	if got.Status != http.StatusNotFound || got.Body != `{"error":"missing"}` {
		t.Fatalf("ToolOutput = %#v", got)
	}
}

func TestServerConvertsUpstreamFailureToToolError(t *testing.T) {
	transportErr := errors.New("transport failed")
	client := &http.Client{Transport: mcpRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}

	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    "http://example.test/customers",
	}, client)
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
		t.Fatalf("CallTool() protocol error = %v", err)
	}
	if !result.IsError || !strings.Contains(firstText(t, result.Content), "transport failed") {
		t.Fatalf("CallTool() result = %#v", result)
	}
}

func TestNewRejectsInvalidInputs(t *testing.T) {
	validOperation := oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    "http://example.test/customers",
	}

	if _, err := New(validOperation, nil); err == nil || !strings.Contains(err.Error(), "HTTP client is required") {
		t.Fatalf("nil-client error = %v", err)
	}

	invalidOperation := validOperation
	invalidOperation.OperationID = "invalid tool"
	if _, err := New(invalidOperation, http.DefaultClient); err == nil ||
		!strings.Contains(err.Error(), "is not a valid MCP tool name") {
		t.Fatalf("invalid-name error = %v", err)
	}
}

func TestToolDescription(t *testing.T) {
	tests := []struct {
		name      string
		operation oasopenapi.SelectedOperation
		want      string
	}{
		{
			name: "summary and description",
			operation: oasopenapi.SelectedOperation{
				Summary:     "List customers",
				Description: "Returns customers.",
			},
			want: "List customers\n\nReturns customers.",
		},
		{
			name:      "description only",
			operation: oasopenapi.SelectedOperation{Description: "Service health."},
			want:      "Service health.",
		},
		{
			name:      "fallback",
			operation: oasopenapi.SelectedOperation{Method: http.MethodGet, Path: "/health"},
			want:      "Call GET /health",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := toolDescription(test.operation); got != test.want {
				t.Fatalf("toolDescription() = %q, want %q", got, test.want)
			}
		})
	}
}

func connectClient(t *testing.T, server *mcp.Server) (*mcp.ClientSession, func()) {
	t.Helper()

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "oasrelay-test", Version: "0.0.0-test"},
		nil,
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}

	cleanup := func() {
		if err := clientSession.Close(); err != nil {
			t.Errorf("clientSession.Close() error = %v", err)
		}
		if err := serverSession.Wait(); err != nil {
			t.Errorf("serverSession.Wait() error = %v", err)
		}
	}
	return clientSession, cleanup
}

func assertObjectSchema(t *testing.T, schema any) {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}
	if object["type"] != "object" {
		t.Fatalf("input schema = %s, want object type", encoded)
	}
}

func decodeToolOutput(t *testing.T, structured any) ToolOutput {
	t.Helper()
	encoded, err := json.Marshal(structured)
	if err != nil {
		t.Fatalf("marshal structured output: %v", err)
	}
	var output ToolOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("unmarshal structured output: %v", err)
	}
	return output
}

func firstText(t *testing.T, content []mcp.Content) string {
	t.Helper()
	if len(content) == 0 {
		t.Fatal("tool result has no content")
	}
	text, ok := content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("first content type = %T, want *mcp.TextContent", content[0])
	}
	return text.Text
}

type mcpRoundTripFunc func(*http.Request) (*http.Response, error)

func (function mcpRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func ExampleToolOutput() {
	output := ToolOutput{Status: 200, ContentType: "application/json", Body: `{}`}
	fmt.Println(output.Status, output.ContentType, output.Body)
	// Output: 200 application/json {}
}

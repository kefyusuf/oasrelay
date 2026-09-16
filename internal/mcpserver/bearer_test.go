package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerWithBearerTokenAuthenticatesUpstreamWithoutExposingSecretInToolSchema(t *testing.T) {
	authorization := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorization <- request.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	operation := oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    upstream.URL + "/customers",
	}
	server, err := NewWithBearerToken(operation, upstream.Client(), "secret-token")
	if err != nil {
		t.Fatalf("NewWithBearerToken() error = %v", err)
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
	encodedSchema, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	for _, forbidden := range []string{"secret-token", "Authorization", "OASRELAY_BEARER_TOKEN"} {
		if strings.Contains(string(encodedSchema), forbidden) {
			t.Fatalf("input schema unexpectedly contains %q: %s", forbidden, encodedSchema)
		}
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "listCustomers",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
	}
	if got := <-authorization; got != "Bearer secret-token" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer secret-token")
	}
}

func TestServerWithoutBearerTokenRemainsUnauthenticated(t *testing.T) {
	authorization := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorization <- request.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	server, err := New(oasopenapi.SelectedOperation{
		OperationID: "listCustomers",
		Method:      http.MethodGet,
		Path:        "/customers",
		Endpoint:    upstream.URL + "/customers",
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
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
	}
	if got := <-authorization; got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
}

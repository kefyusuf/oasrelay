package mcpserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kefyusuf/oasrelay/internal/upstream"
)

func TestRuntimeHTTPClientReadsBearerTokenFromEnvironment(t *testing.T) {
	authorization := make(chan string, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorization <- request.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	originalDefaultTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() {
		http.DefaultTransport = originalDefaultTransport
	})

	client, err := runtimeHTTPClient(func(name string) string {
		if name != "OASRELAY_BEARER_TOKEN" {
			t.Fatalf("environment lookup = %q, want OASRELAY_BEARER_TOKEN", name)
		}
		return "runtime-secret"
	})
	if err != nil {
		t.Fatalf("runtimeHTTPClient() error = %v", err)
	}

	if _, err := upstream.Get(context.Background(), client, server.URL); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got := <-authorization; got != "Bearer runtime-secret" {
		t.Fatalf("Authorization = %q, want bearer header", got)
	}
}

func TestRuntimeHTTPClientRejectsInvalidBearerTokenWithoutLeakingIt(t *testing.T) {
	secret := "runtime-secret\r\nX-Leak: yes"
	_, err := runtimeHTTPClient(func(string) string { return secret })
	if err == nil {
		t.Fatal("runtimeHTTPClient() error = nil, want rejection")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "runtime-secret") {
		t.Fatalf("error %q leaked bearer token", err)
	}
}

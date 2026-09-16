package upstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithBearerTokenAddsAuthorizationHeader(t *testing.T) {
	authorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorization <- request.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	client, err := WithBearerToken(server.Client(), "secret-token")
	if err != nil {
		t.Fatalf("WithBearerToken() error = %v", err)
	}
	if _, err := Get(context.Background(), client, server.URL); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got := <-authorization; got != "Bearer secret-token" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer secret-token")
	}
}

func TestWithBearerTokenTreatsEmptyAndWhitespaceOnlyAsUnset(t *testing.T) {
	for _, token := range []string{"", "   \t  "} {
		t.Run(strings.ReplaceAll(token, " ", "space"), func(t *testing.T) {
			authorization := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				authorization <- request.Header.Get("Authorization")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer server.Close()

			client, err := WithBearerToken(server.Client(), token)
			if err != nil {
				t.Fatalf("WithBearerToken() error = %v", err)
			}
			if _, err := Get(context.Background(), client, server.URL); err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got := <-authorization; got != "" {
				t.Fatalf("Authorization = %q, want empty", got)
			}
		})
	}
}

func TestWithBearerTokenRejectsLineBreaksWithoutEchoingSecret(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "secret with CRLF", token: "top-secret\r\nX-Leak: yes"},
		{name: "line breaks only", token: "\r\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := WithBearerToken(http.DefaultClient, test.token)
			if err == nil {
				t.Fatal("WithBearerToken() error = nil, want rejection")
			}
			if strings.Contains(err.Error(), test.token) || strings.Contains(err.Error(), "top-secret") {
				t.Fatalf("error %q leaked bearer token", err)
			}
		})
	}
}

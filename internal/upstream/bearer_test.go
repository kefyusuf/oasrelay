package upstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestWithBearerTokenDoesNotFollowCrossOriginRedirect(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer target.Close()

	sourceAuthorization := make(chan string, 1)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		sourceAuthorization <- request.Header.Get("Authorization")
		http.Redirect(w, request, target.URL, http.StatusFound)
	}))
	defer source.Close()

	client, err := WithBearerToken(source.Client(), "secret-token")
	if err != nil {
		t.Fatalf("WithBearerToken() error = %v", err)
	}

	response, err := client.Get(source.URL)
	if err != nil {
		t.Fatalf("GET redirect source: %v", err)
	}
	defer response.Body.Close()

	if got := <-sourceAuthorization; got != "Bearer secret-token" {
		t.Fatalf("source Authorization = %q, want bearer header", got)
	}
	if response.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusFound)
	}
	if targetCalls.Load() != 0 {
		t.Fatalf("cross-origin redirect target calls = %d, want 0", targetCalls.Load())
	}
}

func TestWithBearerTokenFollowsSameOriginRedirect(t *testing.T) {
	authorizations := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorizations <- request.Header.Get("Authorization")
		if request.URL.Path == "/start" {
			http.Redirect(w, request, "/final", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	client, err := WithBearerToken(server.Client(), "secret-token")
	if err != nil {
		t.Fatalf("WithBearerToken() error = %v", err)
	}

	response, err := client.Get(server.URL + "/start")
	if err != nil {
		t.Fatalf("GET same-origin redirect: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	for index := 0; index < 2; index++ {
		if got := <-authorizations; got != "Bearer secret-token" {
			t.Fatalf("Authorization[%d] = %q, want bearer header", index, got)
		}
	}
}

package upstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGetWithBearerAddsExactAuthorizationHeader(t *testing.T) {
	header := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		header <- request.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	_, err := GetWithBearer(context.Background(), server.Client(), server.URL, "secret-token")
	if err != nil {
		t.Fatalf("GetWithBearer() error = %v", err)
	}
	if got := <-header; got != "Bearer secret-token" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer secret-token")
	}
}

func TestGetWithBearerOmitsAuthorizationForEmptyToken(t *testing.T) {
	header := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		header <- request.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	_, err := GetWithBearer(context.Background(), server.Client(), server.URL, "")
	if err != nil {
		t.Fatalf("GetWithBearer() error = %v", err)
	}
	if got := <-header; got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
}

func TestGetWithBearerDoesNotFollowRedirect(t *testing.T) {
	var redirectedCalls atomic.Int32
	redirectedAuthorization := make(chan string, 1)
	redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		redirectedCalls.Add(1)
		redirectedAuthorization <- request.Header.Get("Authorization")
		_, _ = io.WriteString(w, "redirect target")
	}))
	defer redirected.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, redirected.URL+"/target", http.StatusFound)
	}))
	defer origin.Close()

	got, err := GetWithBearer(context.Background(), origin.Client(), origin.URL, "secret-token")
	if err != nil {
		t.Fatalf("GetWithBearer() error = %v", err)
	}
	if got.Status != http.StatusFound {
		t.Fatalf("Status = %d, want %d", got.Status, http.StatusFound)
	}
	if redirectedCalls.Load() != 0 {
		t.Fatalf("redirect target calls = %d, want 0", redirectedCalls.Load())
	}
	select {
	case auth := <-redirectedAuthorization:
		t.Fatalf("redirect target observed Authorization %q", auth)
	default:
	}
}

func TestGetWithBearerDoesNotMutateCallerClient(t *testing.T) {
	originalRedirect := func(_ *http.Request, _ []*http.Request) error { return nil }
	client := &http.Client{
		Transport:     http.DefaultTransport,
		CheckRedirect: originalRedirect,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	_, err := GetWithBearer(context.Background(), client, server.URL, "secret-token")
	if err != nil {
		t.Fatalf("GetWithBearer() error = %v", err)
	}
	if client.CheckRedirect == nil {
		t.Fatal("caller CheckRedirect was cleared")
	}

	request, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	if err := client.CheckRedirect(request, nil); err != nil {
		t.Fatalf("caller CheckRedirect changed behavior: %v", err)
	}
}

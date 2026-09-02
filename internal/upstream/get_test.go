package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGetReturnsRawHTTPResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/api/customers" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer server.Close()

	got, err := Get(context.Background(), server.Client(), server.URL+"/api/customers")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	want := Response{
		Status:      http.StatusOK,
		ContentType: "application/json",
		Body:        `{"items":[]}`,
	}
	if got != want || calls.Load() != 1 {
		t.Fatalf("Response = %#v, calls = %d, want %#v and 1 call", got, calls.Load(), want)
	}
}

func TestGetPreservesNon2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "missing")
	}))
	defer server.Close()

	got, err := Get(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	want := Response{
		Status:      http.StatusNotFound,
		ContentType: "text/plain; charset=utf-8",
		Body:        "missing",
	}
	if got != want {
		t.Fatalf("Response = %#v, want %#v", got, want)
	}
}

func TestGetRejectsOversizedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", int(MaxResponseBytes)+1))
	}))
	defer server.Close()

	_, err := Get(context.Background(), server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "response body exceeds 1048576 bytes") {
		t.Fatalf("error = %v, want body-size context", err)
	}
}

func TestGetHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})}

	_, err := Get(ctx, client, "http://example.test")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestGetPropagatesTransportError(t *testing.T) {
	transportErr := errors.New("transport failed")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}

	_, err := Get(context.Background(), client, "http://example.test")
	if !errors.Is(err, transportErr) {
		t.Fatalf("error = %v, want transport error", err)
	}
}

func TestGetRejectsNilClient(t *testing.T) {
	_, err := Get(context.Background(), nil, "http://example.test")
	if err == nil || !strings.Contains(err.Error(), "HTTP client is required") {
		t.Fatalf("error = %v, want nil-client context", err)
	}
}

func TestGetClosesResponseBody(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "success", content: "ok"},
		{name: "oversized", content: strings.Repeat("x", int(MaxResponseBytes)+1), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackingBody{Reader: strings.NewReader(test.content)}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       body,
				}, nil
			})}

			_, err := Get(context.Background(), client, "http://example.test")
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if !body.closed.Load() {
				t.Fatal("response body was not closed")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type trackingBody struct {
	io.Reader
	closed atomic.Bool
}

func (body *trackingBody) Close() error {
	body.closed.Store(true)
	return nil
}

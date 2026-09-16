package upstream

import (
	"fmt"
	"net/http"
	"strings"
)

// WithBearerToken returns a shallow copy of client whose transport adds one
// Authorization bearer header. Empty or whitespace-only tokens leave the
// client unchanged.
func WithBearerToken(client *http.Client, token string) (*http.Client, error) {
	if client == nil {
		return nil, fmt.Errorf("HTTP client is required")
	}
	if strings.TrimSpace(token) == "" {
		return client, nil
	}
	if strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("bearer token contains a prohibited line break")
	}

	clone := *client
	base := clone.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	clone.Transport = bearerTransport{base: base, token: token}
	return &clone, nil
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(clone)
}

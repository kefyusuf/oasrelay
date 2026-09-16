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
	if strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("bearer token contains a prohibited line break")
	}
	if strings.TrimSpace(token) == "" {
		return client, nil
	}

	clone := *client
	base := clone.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	clone.Transport = bearerTransport{base: base, token: token}

	originalCheckRedirect := clone.CheckRedirect
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > 0 {
			origin := via[0].URL
			if !strings.EqualFold(request.URL.Scheme, origin.Scheme) ||
				!strings.EqualFold(request.URL.Host, origin.Host) {
				return http.ErrUseLastResponse
			}
		}

		if originalCheckRedirect != nil {
			return originalCheckRedirect(request, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}

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

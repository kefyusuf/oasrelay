package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const MaxResponseBytes int64 = 1 << 20
const RequestTimeout = 15 * time.Second

// Response is the raw bounded HTTP response exposed by the first MCP tool.
type Response struct {
	Status      int
	ContentType string
	Body        string
}

// Get sends one unauthenticated context-aware HTTP GET request.
func Get(ctx context.Context, client *http.Client, endpoint string) (Response, error) {
	return get(ctx, client, endpoint, "")
}

// GetWithBearer sends one context-aware HTTP GET request with an optional
// process-supplied Bearer token. Authenticated requests stop at redirects so the
// Authorization header cannot be forwarded to a different location.
func GetWithBearer(
	ctx context.Context,
	client *http.Client,
	endpoint, token string,
) (Response, error) {
	return get(ctx, client, endpoint, token)
}

func get(
	ctx context.Context,
	client *http.Client,
	endpoint, bearerToken string,
) (Response, error) {
	if client == nil {
		return Response{}, fmt.Errorf("HTTP client is required")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Response{}, fmt.Errorf("create GET request: %w", err)
	}
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	requestClient := client
	if bearerToken != "" {
		clientCopy := *client
		clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		requestClient = &clientCopy
	}

	response, err := requestClient.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("execute GET request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(body)) > MaxResponseBytes {
		return Response{}, fmt.Errorf("response body exceeds %d bytes", MaxResponseBytes)
	}

	return Response{
		Status:      response.StatusCode,
		ContentType: response.Header.Get("Content-Type"),
		Body:        string(body),
	}, nil
}

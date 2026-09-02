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

// Get sends one context-aware HTTP GET request and reads at most one MiB of
// response data. Non-2xx responses remain ordinary Response values so the MCP
// layer can preserve their status and body while marking the tool result as an
// error.
func Get(ctx context.Context, client *http.Client, endpoint string) (Response, error) {
	if client == nil {
		return Response{}, fmt.Errorf("HTTP client is required")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Response{}, fmt.Errorf("create GET request: %w", err)
	}

	response, err := client.Do(request)
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

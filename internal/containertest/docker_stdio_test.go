//go:build container

package containertest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const expectedRuntimeUser = "65532:65532"

type toolOutput struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Body        string `json:"body"`
}

func TestDockerImageRunsOneToolOverStdio(t *testing.T) {
	image := strings.TrimSpace(os.Getenv("OASRELAY_IMAGE"))
	if image == "" {
		t.Fatal("OASRELAY_IMAGE must name the prebuilt image under test")
	}

	assertImageUser(t, image)

	var calls atomic.Int32
	requests := make(chan string, 1)
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen for upstream fixture: %v", err)
	}

	upstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		select {
		case requests <- request.Method + " " + request.URL.Path:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[]}`)
	})}
	go func() {
		_ = upstream.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = upstream.Shutdown(ctx)
	})

	port := listener.Addr().(*net.TCPAddr).Port
	specPath := writeSpec(t, port)
	mount := fmt.Sprintf(
		"type=bind,src=%s,dst=/work/openapi.yaml,readonly",
		specPath,
	)

	command := exec.Command(
		"docker",
		"run",
		"--rm",
		"-i",
		"--add-host",
		"host.docker.internal:host-gateway",
		"--mount",
		mount,
		image,
		"serve",
		"--operation-id",
		"listCustomers",
		"/work/openapi.yaml",
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "oasrelay-container-test", Version: "0.0.0-test"},
		nil,
	)
	session, err := client.Connect(ctx, &mcp.CommandTransport{
		Command:           command,
		TerminateDuration: 2 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("connect to container MCP server: %v; stderr = %q", err, stderr.String())
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = session.Close()
		}
	})

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list container tools: %v; stderr = %q", err, stderr.String())
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "listCustomers" {
		t.Fatalf("listed tools = %#v, want exactly listCustomers", listed.Tools)
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "listCustomers",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call container tool: %v; stderr = %q", err, stderr.String())
	}
	if result.IsError {
		t.Fatalf("container tool returned an error: %#v", result.Content)
	}

	got := decodeOutput(t, result.StructuredContent)
	want := toolOutput{
		Status:      http.StatusOK,
		ContentType: "application/json",
		Body:        `{"items":[]}`,
	}
	if got != want {
		t.Fatalf("tool output = %#v, want %#v", got, want)
	}

	select {
	case request := <-requests:
		if request != "GET /api/customers" {
			t.Fatalf("upstream request = %q, want %q", request, "GET /api/customers")
		}
	case <-ctx.Done():
		t.Fatalf("upstream request was not observed: %v", ctx.Err())
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls.Load())
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close container MCP session: %v; stderr = %q", err, stderr.String())
	}
	closed = true
}

func assertImageUser(t *testing.T, image string) {
	t.Helper()

	output, err := exec.Command(
		"docker",
		"image",
		"inspect",
		"--format",
		"{{.Config.User}}",
		image,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect image user: %v; output = %q", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != expectedRuntimeUser {
		t.Fatalf("image user = %q, want %q", got, expectedRuntimeUser)
	}
}

func writeSpec(t *testing.T, port int) string {
	t.Helper()

	directory := t.TempDir()
	path := filepath.Join(directory, "openapi.yaml")
	content := fmt.Sprintf(`openapi: 3.0.3
info:
  title: Docker Acceptance API
  version: 1.0.0
servers:
  - url: http://host.docker.internal:%d/api
paths:
  /customers:
    get:
      operationId: listCustomers
      summary: List customers
      responses:
        "200":
          description: Customer collection
`, port)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write OpenAPI fixture: %v", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve OpenAPI fixture path: %v", err)
	}
	return absolute
}

func decodeOutput(t *testing.T, structured any) toolOutput {
	t.Helper()

	encoded, err := json.Marshal(structured)
	if err != nil {
		t.Fatalf("marshal structured output: %v", err)
	}
	var output toolOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("unmarshal structured output: %v", err)
	}
	return output
}

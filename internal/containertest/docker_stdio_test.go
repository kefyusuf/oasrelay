//go:build container

package containertest

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
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
const containerBearerToken = "container-secret"

type toolOutput struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Body        string `json:"body"`
}

type observedRequest struct {
	requestURI    string
	authorization string
}

func TestDockerImageRunsOneToolOverStdio(t *testing.T) {
	image := strings.TrimSpace(os.Getenv("OASRELAY_IMAGE"))
	if image == "" {
		t.Fatal("OASRELAY_IMAGE must name the prebuilt image under test")
	}

	assertImageUser(t, image)

	var calls atomic.Int32
	requests := make(chan observedRequest, 2)
	listener, caPath := listenTLSFixture(t)

	upstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		select {
		case requests <- observedRequest{
			requestURI:    request.Method + " " + request.URL.RequestURI(),
			authorization: request.Header.Get("Authorization"),
		}:
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
	caMount := fmt.Sprintf(
		"type=bind,src=%s,dst=/work/oasrelay-test-ca.pem,readonly",
		caPath,
	)

	command := exec.Command(
		"docker",
		"run",
		"--rm",
		"-i",
		"-e",
		"OASRELAY_BEARER_TOKEN="+containerBearerToken,
		"-e",
		"SSL_CERT_FILE=/work/oasrelay-test-ca.pem",
		"--add-host",
		"host.docker.internal:host-gateway",
		"--mount",
		mount,
		"--mount",
		caMount,
		image,
		"serve",
		"--operation-id",
		"getCustomerOrders",
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
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "getCustomerOrders" {
		t.Fatalf("listed tools = %#v, want exactly getCustomerOrders", listed.Tools)
	}
	listedJSON, err := json.Marshal(listed)
	if err != nil {
		t.Fatalf("marshal listed tools: %v", err)
	}
	if strings.Contains(string(listedJSON), containerBearerToken) {
		t.Fatal("bearer token leaked into MCP tools/list output")
	}

	want := toolOutput{
		Status:      http.StatusOK,
		ContentType: "application/json",
		Body:        `{"items":[]}`,
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "getCustomerOrders",
		Arguments: map[string]any{
			"customerId": "cus_123",
		},
	})
	if err != nil {
		t.Fatalf("call container tool without optional query: %v; stderr = %q", err, stderr.String())
	}
	if result.IsError {
		t.Fatalf("container tool without optional query returned an error: %#v", result.Content)
	}
	if got := decodeOutput(t, result.StructuredContent); got != want {
		t.Fatalf("tool output without optional query = %#v, want %#v", got, want)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal tool result without optional query: %v", err)
	}
	if strings.Contains(string(resultJSON), containerBearerToken) {
		t.Fatal("bearer token leaked into MCP tool output without optional query")
	}

	select {
	case request := <-requests:
		if request.requestURI != "GET /api/customers/cus_123/orders" {
			t.Fatalf(
				"upstream request without optional query = %q, want %q",
				request.requestURI,
				"GET /api/customers/cus_123/orders",
			)
		}
		if request.authorization != "Bearer "+containerBearerToken {
			t.Fatalf("Authorization without optional query = %q, want bearer header", request.authorization)
		}
	case <-ctx.Done():
		t.Fatalf("upstream request without optional query was not observed: %v", ctx.Err())
	}

	result, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "getCustomerOrders",
		Arguments: map[string]any{
			"customerId": "cus_123",
			"limit":      25,
		},
	})
	if err != nil {
		t.Fatalf("call container tool with optional query: %v; stderr = %q", err, stderr.String())
	}
	if result.IsError {
		t.Fatalf("container tool with optional query returned an error: %#v", result.Content)
	}
	if got := decodeOutput(t, result.StructuredContent); got != want {
		t.Fatalf("tool output with optional query = %#v, want %#v", got, want)
	}
	resultJSON, err = json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal tool result with optional query: %v", err)
	}
	if strings.Contains(string(resultJSON), containerBearerToken) {
		t.Fatal("bearer token leaked into MCP tool output with optional query")
	}

	select {
	case request := <-requests:
		if request.requestURI != "GET /api/customers/cus_123/orders?limit=25" {
			t.Fatalf(
				"upstream request with optional query = %q, want %q",
				request.requestURI,
				"GET /api/customers/cus_123/orders?limit=25",
			)
		}
		if request.authorization != "Bearer "+containerBearerToken {
			t.Fatalf("Authorization with optional query = %q, want bearer header", request.authorization)
		}
	case <-ctx.Done():
		t.Fatalf("upstream request with optional query was not observed: %v", ctx.Err())
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls.Load())
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close container MCP session: %v; stderr = %q", err, stderr.String())
	}
	closed = true
	if strings.Contains(stderr.String(), containerBearerToken) {
		t.Fatal("bearer token leaked to container stderr")
	}
}

func TestDockerImageRunsTwoQueryToolOverStdio(t *testing.T) {
	image := strings.TrimSpace(os.Getenv("OASRELAY_IMAGE"))
	if image == "" {
		t.Fatal("OASRELAY_IMAGE must name the prebuilt image under test")
	}
	assertImageUser(t, image)

	var calls atomic.Int32
	requests := make(chan observedRequest, 2)
	listener, caPath := listenTLSFixture(t)
	upstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		select {
		case requests <- observedRequest{
			requestURI:    request.Method + " " + request.URL.RequestURI(),
			authorization: request.Header.Get("Authorization"),
		}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[]}`)
	})}
	go func() { _ = upstream.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = upstream.Shutdown(ctx)
	})

	specPath := writeTwoQuerySpec(t, listener.Addr().(*net.TCPAddr).Port)
	command := exec.Command(
		"docker", "run", "--rm", "-i",
		"-e", "OASRELAY_BEARER_TOKEN="+containerBearerToken,
		"-e", "SSL_CERT_FILE=/work/oasrelay-test-ca.pem",
		"--add-host", "host.docker.internal:host-gateway",
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/work/openapi.yaml,readonly", specPath),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/work/oasrelay-test-ca.pem,readonly", caPath),
		image, "serve", "--operation-id", "listCustomers", "/work/openapi.yaml",
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(
		&mcp.Implementation{Name: "oasrelay-two-query-container-test", Version: "0.0.0-test"}, nil,
	)
	session, err := client.Connect(ctx, &mcp.CommandTransport{
		Command: command, TerminateDuration: 2 * time.Second,
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
	listedJSON, err := json.Marshal(listed)
	if err != nil {
		t.Fatalf("marshal listed tools: %v", err)
	}
	if strings.Contains(string(listedJSON), containerBearerToken) {
		t.Fatal("bearer token leaked into MCP tools/list output")
	}

	want := toolOutput{Status: http.StatusOK, ContentType: "application/json", Body: `{"items":[]}`}
	for _, tc := range []struct {
		name      string
		arguments map[string]any
		request   string
	}{
		{"optional query omitted", map[string]any{"limit": 25}, "GET /api/customers?limit=25"},
		{"optional query supplied", map[string]any{"limit": 25, "cursor": "next page"}, "GET /api/customers?limit=25&cursor=next+page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "listCustomers", Arguments: tc.arguments})
			if err != nil {
				t.Fatalf("call container tool: %v; stderr = %q", err, stderr.String())
			}
			if result.IsError {
				t.Fatalf("container tool returned an error: %#v", result.Content)
			}
			if got := decodeOutput(t, result.StructuredContent); got != want {
				t.Fatalf("tool output = %#v, want %#v", got, want)
			}
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("marshal tool result: %v", err)
			}
			if strings.Contains(string(resultJSON), containerBearerToken) {
				t.Fatal("bearer token leaked into MCP tool output")
			}
			select {
			case request := <-requests:
				if request.requestURI != tc.request {
					t.Fatalf("upstream request = %q, want %q", request.requestURI, tc.request)
				}
				if request.authorization != "Bearer "+containerBearerToken {
					t.Fatal("upstream Authorization does not match the configured bearer header")
				}
			case <-ctx.Done():
				t.Fatalf("upstream request was not observed: %v", ctx.Err())
			}
		})
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls.Load())
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close container MCP session: %v; stderr = %q", err, stderr.String())
	}
	closed = true
	if strings.Contains(stderr.String(), containerBearerToken) {
		t.Fatal("bearer token leaked to container stderr")
	}
}

func writeTwoQuerySpec(t *testing.T, port int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	content := fmt.Sprintf(`openapi: 3.0.3
info:
  title: Two Query Docker Acceptance API
  version: 1.0.0
servers:
  - url: https://host.docker.internal:%d/api
paths:
  /customers:
    get:
      operationId: listCustomers
      parameters:
        - name: limit
          in: query
          required: true
          schema:
            type: integer
        - name: cursor
          in: query
          required: false
          schema:
            type: string
      responses:
        "200":
          description: Customer collection
`, port)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write two-query OpenAPI fixture: %v", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve two-query OpenAPI fixture path: %v", err)
	}
	return absolute
}

func listenTLSFixture(t *testing.T) (net.Listener, string) {
	t.Helper()

	now := time.Now()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate test CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "OASRelay Docker Test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(
		rand.Reader,
		caTemplate,
		caTemplate,
		&caKey.PublicKey,
		caKey,
	)
	if err != nil {
		t.Fatalf("create test CA certificate: %v", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate TLS server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "host.docker.internal"},
		DNSNames:     []string{"host.docker.internal"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(
		rand.Reader,
		serverTemplate,
		caTemplate,
		&serverKey.PublicKey,
		caKey,
	)
	if err != nil {
		t.Fatalf("create TLS server certificate: %v", err)
	}

	baseListener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen for TLS upstream fixture: %v", err)
	}

	caPath := filepath.Join(t.TempDir(), "oasrelay-test-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		_ = baseListener.Close()
		t.Fatalf("write test CA certificate: %v", err)
	}
	absoluteCAPath, err := filepath.Abs(caPath)
	if err != nil {
		_ = baseListener.Close()
		t.Fatalf("resolve test CA certificate path: %v", err)
	}

	listener := tls.NewListener(baseListener, &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverDER, caDER},
			PrivateKey:  serverKey,
		}},
		MinVersion: tls.VersionTLS12,
	})
	return listener, absoluteCAPath
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
  - url: https://host.docker.internal:%d/api
paths:
  /customers/{customerId}/orders:
    get:
      operationId: getCustomerOrders
      summary: Get customer orders
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: limit
          in: query
          required: false
          schema:
            type: integer
      responses:
        "200":
          description: Customer orders
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

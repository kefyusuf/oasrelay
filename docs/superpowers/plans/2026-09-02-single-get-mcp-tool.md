# Single GET MCP Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `oasrelay serve --operation-id <id> <local-spec-path>` that exposes one parameterless OpenAPI `GET` operation as one stdio MCP tool and executes one bounded upstream HTTP request per tool call.

**Architecture:** Extract the existing OpenAPI loader into one shared internal helper, then add a restricted operation selector, a bounded HTTP GET executor, and one official-SDK MCP server adapter. The CLI wires these units together while preserving the existing `inspect` command and reserving stdout exclusively for MCP protocol traffic.

**Tech Stack:** Go 1.25+, Go standard library, `github.com/getkin/kin-openapi` v0.149.0, official `github.com/modelcontextprotocol/go-sdk` v1.7.0.

**Spec:** `docs/superpowers/specs/2026-09-02-single-get-mcp-tool-design.md`

## Global Constraints

- Implement only Issue #4.
- Keep `oasrelay inspect <local-spec-path>` behavior byte-for-byte compatible with its current successful report tests.
- Read OpenAPI only from one local YAML or JSON file.
- Keep external file and URL `$ref` resolution disabled.
- Expose exactly one operation and exactly one stdio MCP tool.
- Accept only a parameterless `GET` operation with no request body.
- Use operation-, path-, then document-level first-server precedence.
- Accept only static absolute `http` or `https` server URLs with no variables.
- Reject an `operationId` that is not a valid MCP tool name: 1–128 characters from `[A-Za-z0-9_.-]`.
- Preserve the selected operation's exact `operationId`; do not sanitize or rename it.
- Use fixed limits: 15-second HTTP client timeout and 1 MiB response body.
- Return non-2xx status/body as structured output with `IsError: true`.
- Do not add parameters, authentication, retries, multiple tools, HTTP MCP transport, remote specs, Docker, configuration, policy, persistence, generation, UI, or SaaS code.

## File Map

- `internal/openapi/load.go`: shared local load, version check, reference policy, and validation.
- `internal/openapi/inspect.go`: existing inspection projection using `loadDocument`.
- `internal/openapi/select.go`: restricted operation selection and endpoint construction.
- `internal/openapi/select_test.go`: selection, server precedence, endpoint, and rejection tests.
- `internal/upstream/get.go`: bounded context-aware HTTP GET.
- `internal/upstream/get_test.go`: real and synthetic HTTP behavior tests.
- `internal/mcpserver/server.go`: one-tool official MCP server and stdio runner.
- `internal/mcpserver/server_test.go`: in-memory MCP list/call integration tests.
- `cmd/oasrelay/main.go`: add `serve` dispatch and strict flag handling.
- `cmd/oasrelay/main_test.go`: serve usage, preflight, and delegation tests.
- `testdata/single-get-api.yaml`: valid operations covering supported and rejected shapes.
- `go.mod`, `go.sum`: add official MCP SDK v1.7.0 and resolved checksums.
- `README.md`: document the one supported MCP workflow and its limits.

---

### Task 1: Share OpenAPI loading and select one supported operation

**Files:**
- Create: `internal/openapi/load.go`
- Create: `internal/openapi/select.go`
- Create: `internal/openapi/select_test.go`
- Create: `testdata/single-get-api.yaml`
- Modify: `internal/openapi/inspect.go`
- Test: `internal/openapi/inspect_test.go`

**Interfaces:**
- Consumes: a local spec path and exact `operationId`.
- Produces: `func loadDocument(path string) (*openapi3.T, error)`.
- Produces: `func SelectParameterlessGET(path, operationID string) (SelectedOperation, error)`.
- Produces:

```go
type SelectedOperation struct {
    OperationID string
    Method      string
    Path        string
    Summary     string
    Description string
    Endpoint    string
}
```

- [ ] **Step 1: Add a focused OpenAPI fixture**

Create `testdata/single-get-api.yaml` with unique operations that exercise document-, path-, and operation-level servers plus each rejection path:

```yaml
openapi: 3.0.3
info:
  title: Single Tool API
  version: 1.0.0
servers:
  - url: https://document.example.test/api
paths:
  /customers:
    get:
      operationId: listCustomers
      summary: List customers
      description: Returns the current customer collection.
      responses:
        "200":
          description: Customer list
    post:
      operationId: createCustomer
      responses:
        "201":
          description: Customer created
  /path-scoped:
    servers:
      - url: https://path.example.test/v1
    get:
      operationId: pathScoped
      responses:
        "200":
          description: Path-scoped response
  /operation-scoped:
    servers:
      - url: https://ignored-path.example.test/v1
    get:
      operationId: operationScoped
      servers:
        - url: https://operation.example.test/v2
      responses:
        "200":
          description: Operation-scoped response
  /customers/{customerId}:
    get:
      operationId: getCustomer
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Customer
  /filtered:
    parameters:
      - name: locale
        in: query
        schema:
          type: string
    get:
      operationId: listFiltered
      responses:
        "200":
          description: Filtered list
  /with-body:
    get:
      operationId: getWithBody
      requestBody:
        required: false
        content:
          application/json:
            schema:
              type: object
      responses:
        "200":
          description: Response
  /relative:
    get:
      operationId: relativeServer
      servers:
        - url: /relative-api
      responses:
        "200":
          description: Response
  /variable:
    get:
      operationId: variableServer
      servers:
        - url: https://{tenant}.example.test/api
          variables:
            tenant:
              default: demo
      responses:
        "200":
          description: Response
  /unsupported-scheme:
    get:
      operationId: ftpServer
      servers:
        - url: ftp://example.test/api
      responses:
        "200":
          description: Response
  /invalid-tool-name:
    get:
      operationId: invalid tool
      responses:
        "200":
          description: Response
```

Use temporary documents inside individual tests for a missing server and an operation-level parameter when that produces clearer isolation.

- [ ] **Step 2: Write failing selector tests**

Create `internal/openapi/select_test.go`. Include a helper using the repository fixture and table-driven rejection cases. The success assertions must be exact:

```go
func TestSelectParameterlessGETUsesDocumentServer(t *testing.T) {
    got, err := SelectParameterlessGET(fixturePath("single-get-api.yaml"), "listCustomers")
    if err != nil {
        t.Fatalf("SelectParameterlessGET() error = %v", err)
    }

    want := SelectedOperation{
        OperationID: "listCustomers",
        Method:      http.MethodGet,
        Path:        "/customers",
        Summary:     "List customers",
        Description: "Returns the current customer collection.",
        Endpoint:    "https://document.example.test/api/customers",
    }
    if got != want {
        t.Fatalf("SelectedOperation = %#v, want %#v", got, want)
    }
}

func TestSelectParameterlessGETUsesNarrowestServer(t *testing.T) {
    tests := []struct {
        operationID string
        endpoint    string
    }{
        {"pathScoped", "https://path.example.test/v1/path-scoped"},
        {"operationScoped", "https://operation.example.test/v2/operation-scoped"},
    }
    // Run each case and compare Endpoint exactly.
}
```

Add exact error-substring assertions for:

```text
operationId "missing" was not found
operationId "createCustomer" uses POST; this version supports GET only
operationId "getCustomer" has parameters; this version supports parameterless operations only
operationId "listFiltered" has parameters; this version supports parameterless operations only
operationId "getWithBody" has a request body; this version does not support request bodies
operationId "relativeServer" has no usable absolute HTTP(S) server URL
operationId "variableServer" uses server variables; this version supports static server URLs only
operationId "ftpServer" has no usable absolute HTTP(S) server URL
operationId "invalid tool" is not a valid MCP tool name
```

Also create a temporary valid OpenAPI document with no `servers` entry and assert the missing-server error.

- [ ] **Step 3: Verify the selector RED state**

Run:

```bash
go test ./internal/openapi -run 'TestSelectParameterlessGET'
```

Expected: compilation fails because `SelectedOperation` and `SelectParameterlessGET` do not exist.

- [ ] **Step 4: Extract the existing loader without changing behavior**

Create `internal/openapi/load.go`:

```go
package openapi

import (
    "fmt"

    "github.com/getkin/kin-openapi/openapi3"
)

func loadDocument(path string) (*openapi3.T, error) {
    loader := openapi3.NewLoader()
    loader.IsExternalRefsAllowed = false

    document, err := loader.LoadFromFile(path)
    if err != nil {
        return nil, fmt.Errorf("load document: %w", err)
    }
    if document.OpenAPIMajorMinor() == "" {
        return nil, fmt.Errorf(
            "unsupported OpenAPI version %q: expected a supported 3.x version",
            document.OpenAPI,
        )
    }
    if err := document.Validate(loader.Context); err != nil {
        return nil, fmt.Errorf("validate document: %w", err)
    }
    return document, nil
}
```

Modify `InspectFile` to call `loadDocument(path)` and leave its projection and sorting unchanged. Remove only the imports made obsolete by the extraction.

- [ ] **Step 5: Verify existing inspect behavior after extraction**

Run:

```bash
go test ./internal/openapi -run 'TestInspectFile'
```

Expected: all existing inspection tests pass with unchanged output behavior.

- [ ] **Step 6: Implement the minimum selector**

Create `internal/openapi/select.go` with these helpers and no exported parser types:

```go
package openapi

import (
    "fmt"
    "net/http"
    "net/url"
    "strings"

    "github.com/getkin/kin-openapi/openapi3"
)

// SelectedOperation is the validated project-owned operation required by the
// one-tool MCP runtime.
type SelectedOperation struct {
    OperationID string
    Method      string
    Path        string
    Summary     string
    Description string
    Endpoint    string
}

func SelectParameterlessGET(path, operationID string) (SelectedOperation, error) {
    document, err := loadDocument(path)
    if err != nil {
        return SelectedOperation{}, err
    }

    for route, item := range document.Paths.Map() {
        if item == nil {
            continue
        }
        for method, operation := range item.Operations() {
            if operation == nil || operation.OperationID != operationID {
                continue
            }
            return selectOperation(document, route, method, item, operation)
        }
    }

    return SelectedOperation{}, fmt.Errorf("operationId %q was not found", operationID)
}

func selectOperation(
    document *openapi3.T,
    route, method string,
    item *openapi3.PathItem,
    operation *openapi3.Operation,
) (SelectedOperation, error) {
    id := operation.OperationID
    if method != http.MethodGet {
        return SelectedOperation{}, fmt.Errorf(
            "operationId %q uses %s; this version supports GET only", id, method,
        )
    }
    if len(item.Parameters) != 0 || len(operation.Parameters) != 0 {
        return SelectedOperation{}, fmt.Errorf(
            "operationId %q has parameters; this version supports parameterless operations only", id,
        )
    }
    if operation.RequestBody != nil {
        return SelectedOperation{}, fmt.Errorf(
            "operationId %q has a request body; this version does not support request bodies", id,
        )
    }
    if err := validateToolName(id); err != nil {
        return SelectedOperation{}, fmt.Errorf("operationId %q is not a valid MCP tool name: %w", id, err)
    }

    server := effectiveServer(document, item, operation)
    endpoint, err := endpointFor(server, route)
    if err != nil {
        return SelectedOperation{}, fmt.Errorf("operationId %q: %w", id, err)
    }

    return SelectedOperation{
        OperationID: id,
        Method:      http.MethodGet,
        Path:        route,
        Summary:     operation.Summary,
        Description: operation.Description,
        Endpoint:    endpoint,
    }, nil
}
```

Implement `effectiveServer` using operation → path → document precedence. Implement `validateToolName` as a length and rune loop matching `[A-Za-z0-9_.-]`. Implement endpoint validation and joining:

```go
func endpointFor(server *openapi3.Server, route string) (string, error) {
    if server == nil || server.URL == "" {
        return "", fmt.Errorf("has no usable absolute HTTP(S) server URL")
    }
    if len(server.Variables) != 0 || strings.ContainsAny(server.URL, "{}") {
        return "", fmt.Errorf("uses server variables; this version supports static server URLs only")
    }

    parsed, err := url.Parse(server.URL)
    if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
        return "", fmt.Errorf("has no usable absolute HTTP(S) server URL")
    }

    endpoint, err := url.JoinPath(parsed.String(), strings.TrimPrefix(route, "/"))
    if err != nil {
        return "", fmt.Errorf("build endpoint: %w", err)
    }
    return endpoint, nil
}
```

- [ ] **Step 7: Verify selector GREEN and full OpenAPI package**

Run:

```bash
gofmt -w internal/openapi
go test ./internal/openapi
```

Expected: all selector and pre-existing inspection tests pass.

- [ ] **Step 8: Commit Task 1**

```bash
git add internal/openapi testdata/single-get-api.yaml
git commit -m "feat: select one parameterless GET operation"
```

---

### Task 2: Execute one bounded upstream GET request

**Files:**
- Create: `internal/upstream/get.go`
- Create: `internal/upstream/get_test.go`

**Interfaces:**
- Consumes: context, non-nil `*http.Client`, and validated endpoint.
- Produces: `func Get(ctx context.Context, client *http.Client, endpoint string) (Response, error)`.
- Produces exported fixed constants used by the stdio runner:

```go
const MaxResponseBytes int64 = 1 << 20
const RequestTimeout = 15 * time.Second
```

- [ ] **Step 1: Write failing HTTP behavior tests**

Create `internal/upstream/get_test.go` using `httptest.Server` for observable network behavior:

```go
func TestGetReturnsRawHTTPResponse(t *testing.T) {
    var calls atomic.Int32
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        calls.Add(1)
        if r.Method != http.MethodGet || r.URL.Path != "/api/customers" {
            t.Errorf("request = %s %s", r.Method, r.URL.Path)
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
        t.Fatalf("Response = %#v, calls = %d", got, calls.Load())
    }
}
```

Add tests that:

- preserve a `404` status and raw body without returning an error;
- reject `MaxResponseBytes+1` bytes with an error containing `response body exceeds 1048576 bytes`;
- return `context.Canceled` or an error wrapping it for an already-cancelled context;
- propagate a synthetic transport error;
- reject a nil client;
- prove a synthetic response body is closed after both success and oversized-body paths.

Use a small `roundTripFunc` test type:

```go
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
    return f(request)
}
```

- [ ] **Step 2: Verify HTTP RED state**

Run:

```bash
go test ./internal/upstream
```

Expected: compilation fails because `Get`, `Response`, and the constants do not exist.

- [ ] **Step 3: Implement bounded HTTP GET**

Create `internal/upstream/get.go`:

```go
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

type Response struct {
    Status      int
    ContentType string
    Body        string
}

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
```

- [ ] **Step 4: Verify HTTP GREEN**

Run:

```bash
gofmt -w internal/upstream
go test ./internal/upstream
```

Expected: all upstream tests pass.

- [ ] **Step 5: Commit Task 2**

```bash
git add internal/upstream
git commit -m "feat: execute bounded upstream GET requests"
```

---

### Task 3: Expose the operation through one official-SDK MCP tool

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/mcpserver/server.go`
- Create: `internal/mcpserver/server_test.go`

**Interfaces:**
- Consumes: `openapi.SelectedOperation` and `*http.Client`.
- Produces: `func New(operation openapi.SelectedOperation, client *http.Client) (*mcp.Server, error)`.
- Produces: `func RunStdio(ctx context.Context, operation openapi.SelectedOperation) error`.

- [ ] **Step 1: Add the official SDK dependency declaration**

Run:

```bash
go get github.com/modelcontextprotocol/go-sdk@v1.7.0
```

Do not add third-party MCP wrappers.

- [ ] **Step 2: Write failing in-memory MCP integration tests**

Create `internal/mcpserver/server_test.go`. Build a real server and client through `mcp.NewInMemoryTransports()`:

```go
func connectClient(t *testing.T, server *mcp.Server) (*mcp.ClientSession, func()) {
    t.Helper()

    ctx := context.Background()
    serverTransport, clientTransport := mcp.NewInMemoryTransports()
    serverSession, err := server.Connect(ctx, serverTransport, nil)
    if err != nil {
        t.Fatalf("server.Connect() error = %v", err)
    }

    client := mcp.NewClient(
        &mcp.Implementation{Name: "oasrelay-test", Version: "0.0.0-test"},
        nil,
    )
    clientSession, err := client.Connect(ctx, clientTransport, nil)
    if err != nil {
        t.Fatalf("client.Connect() error = %v", err)
    }

    cleanup := func() {
        if err := clientSession.Close(); err != nil {
            t.Errorf("clientSession.Close() error = %v", err)
        }
        if err := serverSession.Wait(); err != nil {
            t.Errorf("serverSession.Wait() error = %v", err)
        }
    }
    return clientSession, cleanup
}
```

The primary integration test must:

1. start `httptest.Server` and record method/path/call count;
2. create `openapi.SelectedOperation{OperationID: "listCustomers", Method: "GET", Path: "/customers", Summary: "List customers", Endpoint: testServer.URL + "/api/customers"}`;
3. call `New(operation, testServer.Client())`;
4. connect an MCP client;
5. call `ListTools(ctx, nil)` and assert one tool named `listCustomers`, empty-object input schema, and description `List customers`;
6. call `CallTool` with `Arguments: map[string]any{}`;
7. marshal `result.StructuredContent`, unmarshal it into `ToolOutput`, and compare status/content type/body;
8. assert `IsError == false` and exactly one upstream `GET /api/customers` call.

Add a second integration test where upstream returns `404`; assert `CallTool` returns no protocol error, `IsError == true`, and structured status/body are preserved.

Add constructor tests for a nil HTTP client and invalid tool name. Although the OpenAPI selector also validates names, the MCP package must defend its own exported constructor.

- [ ] **Step 3: Verify MCP RED state**

Run:

```bash
go test ./internal/mcpserver
```

Expected: compilation fails because `New`, `ToolOutput`, and `RunStdio` do not exist.

- [ ] **Step 4: Implement one-tool MCP server**

Create `internal/mcpserver/server.go`:

```go
package mcpserver

import (
    "context"
    "fmt"
    "net/http"
    "strings"

    oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
    "github.com/kefyusuf/oasrelay/internal/upstream"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
    implementationName    = "oasrelay"
    implementationVersion = "0.0.0-dev"
)

type ToolInput struct{}

type ToolOutput struct {
    Status      int    `json:"status" jsonschema:"HTTP response status code"`
    ContentType string `json:"contentType" jsonschema:"HTTP Content-Type response header"`
    Body        string `json:"body" jsonschema:"raw HTTP response body"`
}

func New(operation oasopenapi.SelectedOperation, client *http.Client) (*mcp.Server, error) {
    if client == nil {
        return nil, fmt.Errorf("HTTP client is required")
    }
    if err := validateToolName(operation.OperationID); err != nil {
        return nil, fmt.Errorf("operationId %q is not a valid MCP tool name: %w", operation.OperationID, err)
    }

    server := mcp.NewServer(
        &mcp.Implementation{Name: implementationName, Version: implementationVersion},
        nil,
    )

    mcp.AddTool(
        server,
        &mcp.Tool{
            Name:        operation.OperationID,
            Description: toolDescription(operation),
        },
        func(ctx context.Context, _ *mcp.CallToolRequest, _ ToolInput) (*mcp.CallToolResult, ToolOutput, error) {
            response, err := upstream.Get(ctx, client, operation.Endpoint)
            if err != nil {
                return nil, ToolOutput{}, err
            }

            output := ToolOutput{
                Status:      response.Status,
                ContentType: response.ContentType,
                Body:        response.Body,
            }
            if response.Status < http.StatusOK || response.Status >= http.StatusMultipleChoices {
                return &mcp.CallToolResult{IsError: true}, output, nil
            }
            return nil, output, nil
        },
    )

    return server, nil
}

func RunStdio(ctx context.Context, operation oasopenapi.SelectedOperation) error {
    client := &http.Client{Timeout: upstream.RequestTimeout}
    server, err := New(operation, client)
    if err != nil {
        return err
    }
    return server.Run(ctx, &mcp.StdioTransport{})
}
```

Implement `toolDescription` by trimming summary and description, joining two non-empty values with a blank line, and falling back to `Call GET <path>`. Implement local defensive `validateToolName` with the same explicit constraints as the selector; do not expose a generic shared validation package for two short functions.

- [ ] **Step 5: Resolve dependencies and verify MCP GREEN**

Run:

```bash
gofmt -w internal/mcpserver
go mod tidy
go test ./internal/mcpserver
```

Expected: both in-memory list/call integration tests and constructor tests pass.

- [ ] **Step 6: Run all package tests after adding the SDK**

Run:

```bash
go test ./...
go vet ./...
```

Expected: every package passes.

- [ ] **Step 7: Commit Task 3**

```bash
git add go.mod go.sum internal/mcpserver
git commit -m "feat: expose one GET operation as an MCP tool"
```

---

### Task 4: Add strict `serve` CLI dispatch

**Files:**
- Modify: `cmd/oasrelay/main.go`
- Modify: `cmd/oasrelay/main_test.go`

**Interfaces:**
- Consumes: `openapi.SelectParameterlessGET` and `mcpserver.RunStdio`.
- Produces: `oasrelay serve --operation-id <id> <local-spec-path>`.
- Preserves: `func run(args []string, stdout, stderr io.Writer) int`.
- Adds test seam:

```go
type serveFunc func(context.Context, openapi.SelectedOperation) error
func runWithServe(args []string, stdout, stderr io.Writer, serve serveFunc) int
```

- [ ] **Step 1: Write failing CLI tests**

Extend `cmd/oasrelay/main_test.go` with:

```go
func TestRunServeSelectsOperationAndDelegates(t *testing.T) {
    var stdout bytes.Buffer
    var stderr bytes.Buffer
    var selected openapi.SelectedOperation
    calls := 0

    code := runWithServe(
        []string{"serve", "--operation-id", "listCustomers", cliFixturePath("single-get-api.yaml")},
        &stdout,
        &stderr,
        func(_ context.Context, operation openapi.SelectedOperation) error {
            calls++
            selected = operation
            return nil
        },
    )

    if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 || calls != 1 {
        t.Fatalf("code=%d stdout=%q stderr=%q calls=%d", code, stdout.String(), stderr.String(), calls)
    }
    if selected.OperationID != "listCustomers" || selected.Endpoint != "https://document.example.test/api/customers" {
        t.Fatalf("selected = %#v", selected)
    }
}
```

Add usage-error tests for:

- missing `--operation-id`;
- missing file path;
- two file paths;
- unknown flag;
- positional file path before the flag.

Each must return code `2`, leave stdout empty, and contain:

```text
usage: oasrelay serve --operation-id <id> <local-spec-path>
```

Add an operational-error test selecting `createCustomer`; it must return code `1`, leave stdout empty, avoid calling the serve function, and write the method restriction to stderr.

Add a runtime-error test where the injected serve function returns `errors.New("stdio failed")`; it must return code `1`, leave stdout empty, and include `serve MCP server: stdio failed` on stderr.

Keep every existing inspect test unchanged.

- [ ] **Step 2: Verify CLI RED state**

Run:

```bash
go test ./cmd/oasrelay -run 'TestRunServe'
```

Expected: compilation fails because `runWithServe` and serve dispatch do not exist.

- [ ] **Step 3: Implement strict serve parsing and dispatch**

Refactor `cmd/oasrelay/main.go` without changing inspect formatting:

```go
const (
    inspectUsage = "usage: oasrelay inspect <local-spec-path>"
    serveUsage   = "usage: oasrelay serve --operation-id <id> <local-spec-path>"
)

type serveFunc func(context.Context, oasopenapi.SelectedOperation) error

func run(args []string, stdout, stderr io.Writer) int {
    return runWithServe(args, stdout, stderr, mcpserver.RunStdio)
}

func runWithServe(args []string, stdout, stderr io.Writer, serve serveFunc) int {
    if len(args) == 0 {
        fmt.Fprintln(stderr, inspectUsage)
        fmt.Fprintln(stderr, serveUsage)
        return 2
    }

    switch args[0] {
    case "inspect":
        return runInspect(args[1:], stdout, stderr)
    case "serve":
        return runServe(args[1:], stderr, serve)
    default:
        fmt.Fprintf(stderr, "error: unknown command %q\n", args[0])
        fmt.Fprintln(stderr, inspectUsage)
        fmt.Fprintln(stderr, serveUsage)
        return 2
    }
}
```

Implement `runServe` with a `flag.FlagSet` using `flag.ContinueOnError`. Direct parser diagnostics to `io.Discard`, then emit one stable OASRelay diagnostic and `serveUsage`. Require non-empty `--operation-id` and exactly one remaining positional path. Because Go's standard flag parser stops at the first positional argument, the documented flags-before-path contract is enforced naturally.

On valid arguments:

```go
operation, err := oasopenapi.SelectParameterlessGET(specPath, operationID)
if err != nil {
    fmt.Fprintf(stderr, "error: select operation: %v\n", err)
    return 1
}
if err := serve(context.Background(), operation); err != nil {
    fmt.Fprintf(stderr, "error: serve MCP server: %v\n", err)
    return 1
}
return 0
```

Do not write any serve-path message to stdout.

- [ ] **Step 4: Verify CLI GREEN and inspect regression safety**

Run:

```bash
gofmt -w cmd/oasrelay
go test ./cmd/oasrelay
go test ./...
```

Expected: all serve and unchanged inspect tests pass.

- [ ] **Step 5: Commit Task 4**

```bash
git add cmd/oasrelay
git commit -m "feat: add single-tool stdio serve command"
```

---

### Task 5: Document and verify the complete slice

**Files:**
- Modify: `README.md`
- Modify only if required: `.github/workflows/ci.yml`

- [ ] **Step 1: Update README to match implemented behavior**

Add a `Serve one MCP tool` section:

```bash
go run ./cmd/oasrelay serve \
  --operation-id listCustomers \
  ./openapi.yaml
```

State explicitly:

- local OpenAPI file only;
- one exact `operationId`;
- parameterless `GET` only;
- static absolute HTTP(S) server required;
- stdio transport only;
- 15-second timeout;
- 1 MiB response limit;
- tool output fields `status`, `contentType`, and `body`;
- non-2xx responses are MCP tool errors with preserved structured output;
- no auth, parameters, Docker, remote specs, or multiple tools yet.

Preserve the inspect documentation.

- [ ] **Step 2: Ensure CI visibly executes MCP integration tests**

The existing `go test ./...` step already runs `internal/mcpserver` integration tests. Do not duplicate it merely for naming. Change the workflow only if module caching or another actual requirement demands it.

- [ ] **Step 3: Run fresh full verification**

Run exactly:

```bash
go mod tidy
git diff --exit-code -- go.mod go.sum
gofmt -w cmd internal
git diff --exit-code -- '*.go'
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

Then run the focused integration test by exact name to preserve explicit evidence:

```bash
go test ./internal/mcpserver -run TestServerExposesAndCallsOneGETTool -v
```

Expected: all commands exit zero; the inspect smoke output lists exactly `listCustomers` and `getCustomer`; the MCP integration test lists and calls exactly one tool.

- [ ] **Step 4: Perform scope self-review**

Compare the branch against `main` and verify:

- no parameter schema generation;
- no auth or secret code;
- no HTTP MCP listener;
- no multiple-tool loop;
- no Docker files;
- no config, policy, persistence, generation, UI, or SaaS paths;
- no behavior change in `inspect`;
- every new exported function has direct tests;
- every production behavior was introduced after an observed failing test.

Fix critical or important findings before proceeding.

- [ ] **Step 5: Commit documentation**

```bash
git add README.md .github/workflows/ci.yml
git commit -m "docs: explain single GET MCP workflow"
```

Do not stage the workflow when it did not change.

- [ ] **Step 6: Open a non-draft pull request**

Create a PR from `feat/single-get-mcp-tool` to `main` with:

- Issue #4 closure;
- exact supported operation shape;
- TDD red/green evidence;
- full CI and focused MCP integration evidence;
- explicit non-goals and self-review result.

Do not merge the PR automatically until its head SHA has a fresh green CI run.

## Definition of Done

Issue #4 is complete only when the exact CLI contract works, one official-SDK in-memory client can list and call one real upstream-backed tool, all fixed limits and rejection paths are tested, existing inspect behavior remains green, the final CI run is successful on the PR head SHA, and the diff contains no functionality outside this plan.
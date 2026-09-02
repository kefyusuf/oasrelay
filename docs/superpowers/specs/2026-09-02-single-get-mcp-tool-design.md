# Single GET MCP Tool — Design

**Date:** 2026-09-02  
**Status:** Approved baseline  
**Related issue:** #4

## Context

OASRelay can currently validate a local OpenAPI 3.x YAML or JSON document and list its `GET` operations. The next slice must prove the complete product premise without expanding into a general OpenAPI-to-MCP platform.

This slice selects one deliberately restricted OpenAPI operation, publishes exactly one MCP tool over stdio, executes one real upstream HTTP request, and returns a bounded raw response.

## Goal

Provide this command:

```bash
oasrelay serve --operation-id listCustomers ./openapi.yaml
```

For a supported operation, the process starts an MCP server over stdin/stdout. The server exposes one tool named `listCustomers`. Calling that tool sends one HTTP `GET` request and returns:

```json
{
  "status": 200,
  "contentType": "application/json",
  "body": "{\"items\":[]}"
}
```

## Non-goals

This slice does not implement:

- path, query, header, or cookie parameters;
- request bodies;
- authentication or secret handling;
- non-GET operations;
- more than one MCP tool;
- Streamable HTTP MCP transport;
- remote OpenAPI loading;
- base URL overrides;
- Docker packaging;
- configuration or policy files;
- response-schema transformation;
- code generation;
- persistence, UI, tenancy, billing, or SaaS capabilities.

## CLI contract

### Command

```text
oasrelay serve --operation-id <id> <local-spec-path>
```

Flags precede the positional file path. `--operation-id` is required and exactly one local file path is accepted.

The `serve` command must not print banners, reports, or diagnostics to standard output because stdout is reserved for MCP protocol traffic. Startup and validation failures are written to standard error and return a non-zero exit code.

The existing command remains unchanged:

```text
oasrelay inspect <local-spec-path>
```

### Exit codes

- `0`: the MCP stdio session ended normally;
- `1`: OpenAPI selection, endpoint, upstream setup, or MCP runtime failure;
- `2`: invalid command usage.

## Supported OpenAPI operation

The selected operation is found by scanning every operation and comparing its exact `operationId`.

It is accepted only when:

1. its HTTP method is `GET`;
2. the path item has no parameters;
3. the operation has no parameters;
4. the operation has no request body;
5. an effective server URL exists;
6. that server URL is static, absolute, and uses `http` or `https`.

Scanning all methods is intentional: selecting a known `POST` operation must report that it is unsupported rather than incorrectly reporting that the ID does not exist.

### Server precedence

The first server from the narrowest available scope is used:

1. operation-level `servers[0]`;
2. path-level `servers[0]`;
3. document-level `servers[0]`.

A server is rejected when its URL contains template braces, it declares variables, it is relative, it has no host, or its scheme is not `http` or `https`.

The endpoint is the effective server URL joined with the selected OpenAPI path. Because parameterized operations are rejected, no path-template expansion occurs in this slice.

## Architecture

```text
cmd/oasrelay
  command parsing and exit codes
        |
        v
internal/openapi
  shared load + validate
  select one restricted operation
        |
        v
internal/mcpserver
  register exactly one typed MCP tool
        |
        v
internal/upstream
  bounded HTTP GET execution
```

Proposed file changes:

```text
cmd/oasrelay/
├── main.go
└── main_test.go
internal/openapi/
├── load.go
├── inspect.go
├── inspect_test.go
├── select.go
└── select_test.go
internal/upstream/
├── get.go
└── get_test.go
internal/mcpserver/
├── server.go
└── server_test.go
testdata/
└── single-get-api.yaml
go.mod
go.sum
README.md
```

No generic compiler, registry, policy engine, plugin interface, or configuration model is introduced.

## OpenAPI loading boundary

The existing loader and validator logic moves into one unexported helper:

```go
func loadDocument(path string) (*openapi3.T, error)
```

It preserves current behavior:

- local files only;
- YAML and JSON parsing;
- supported OpenAPI 3.x versions only;
- internal references allowed;
- external file and URL references disabled;
- complete OpenAPI validation.

`InspectFile` and operation selection both call this helper. This is a focused extraction to prevent behavior drift, not a new abstraction layer.

## Selected operation model

```go
type SelectedOperation struct {
    OperationID string
    Method      string
    Path        string
    Summary     string
    Description string
    Endpoint    string
}

func SelectParameterlessGET(path, operationID string) (SelectedOperation, error)
```

The function returns a project-owned value and does not expose parser-library types to the MCP or HTTP packages.

Errors identify the selected `operationId` and the failed restriction, for example:

```text
operationId "createCustomer" uses POST; this version supports GET only
operationId "getCustomer" has parameters; this version supports parameterless operations only
operationId "listCustomers" has no usable absolute HTTP(S) server URL
operationId "missing" was not found
```

## Upstream HTTP boundary

```go
type Response struct {
    Status      int
    ContentType string
    Body        string
}

func Get(ctx context.Context, client *http.Client, endpoint string) (Response, error)
```

The production client uses a fixed 15-second timeout. The response body is read through a limit of 1 MiB plus one byte. A body larger than 1 MiB returns an error; OASRelay never reports a truncated body as a successful result.

`Get` always closes the response body. It returns non-2xx responses as normal `Response` values so the MCP layer can preserve status and body while marking the tool result as an error.

No retries, authentication, custom headers, caching, redirect configuration, content decoding, or schema validation are added.

## MCP server boundary

The project uses the official MCP Go SDK `github.com/modelcontextprotocol/go-sdk/mcp` v1.7.0.

```go
type ToolInput struct{}

type ToolOutput struct {
    Status      int    `json:"status"`
    ContentType string `json:"contentType"`
    Body        string `json:"body"`
}

func New(operation openapi.SelectedOperation, client *http.Client) *mcp.Server
func RunStdio(ctx context.Context, operation openapi.SelectedOperation) error
```

`New` registers exactly one tool:

- name: selected `operationId`;
- input: empty object;
- output: `ToolOutput`;
- description: trimmed OpenAPI summary and description, or `Call GET <path>` when both are absent.

The handler calls `upstream.Get` once.

- HTTP 2xx: structured output with a normal MCP result;
- HTTP non-2xx: the same structured output with `IsError: true`;
- transport, timeout, cancellation, read, or size failure: a tool execution error.

`RunStdio` creates the fixed-timeout production HTTP client and runs the server with `mcp.StdioTransport`. It adds no HTTP MCP listener.

## CLI testability

The production `run` function remains available for existing tests. Command dispatch delegates valid `serve` execution through an internal function parameter so CLI tests can verify parsing and selection without opening a blocking stdio session.

Conceptually:

```go
type serveFunc func(context.Context, openapi.SelectedOperation) error

func run(args []string, stdout, stderr io.Writer) int
func runWithServe(args []string, stdout, stderr io.Writer, serve serveFunc) int
```

The default `run` supplies `mcpserver.RunStdio`; focused CLI tests supply a recording fake. This injection exists only at the command boundary and does not create a general dependency container.

## Testing

### OpenAPI selection tests

- select a parameterless `GET` operation;
- preserve summary and description;
- operation-, path-, and document-server precedence;
- operation ID not found;
- known non-GET operation rejected;
- path-level parameters rejected;
- operation-level parameters rejected;
- request body rejected;
- missing server rejected;
- relative server rejected;
- variable server rejected;
- non-HTTP(S) server rejected;
- endpoint path joining.

Existing inspection tests must remain green after loader extraction.

### Upstream tests

Using `httptest.Server` and real HTTP requests:

- sends exactly one `GET` to the expected path;
- returns status, content type, and raw body;
- preserves non-2xx responses;
- rejects responses larger than 1 MiB;
- propagates cancellation and network errors;
- closes response bodies through normal client behavior.

### MCP integration tests

Using the official SDK's in-memory transports:

1. construct an OASRelay server backed by `httptest.Server`;
2. connect one MCP client;
3. list tools and assert exactly one tool with the selected name;
4. call it with an empty object;
5. assert the structured result and upstream method/path;
6. assert non-2xx responses set `IsError` while preserving structured output.

This verifies real MCP list/call behavior without subprocesses or stdin/stdout timing flakiness.

### CLI tests

- required `--operation-id`;
- exactly one local file path;
- unknown flags and excess arguments;
- selection errors return code `1` and write only to stderr;
- valid input delegates the exact selected operation to the injected serve function;
- `inspect` output remains unchanged.

## Definition of done

The slice is complete when:

```bash
go mod tidy
gofmt -w cmd internal
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

all succeed, MCP integration tests prove one real tool call against `httptest.Server`, the final diff stays within Issue #4, and the work is submitted through a separate pull request without adding the next feature.
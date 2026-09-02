# OASRelay OpenAPI Inspect — Design

**Date:** 2026-09-02  
**Status:** Approved baseline  
**Related issue:** #1

## Context

OASRelay will eventually expose selected OpenAPI operations as local MCP tools. The first release must not attempt that full runtime. It starts with the smallest independently testable capability: loading a local OpenAPI document and inspecting its `GET` operations from a command-line interface.

This slice establishes only the OpenAPI loading, validation, discovery, output, and error-handling boundaries required by later work.

## Goal

Provide this command:

```bash
oasrelay inspect ./openapi.yaml
```

For a valid local OpenAPI 3.x YAML or JSON document, the command prints:

- OpenAPI version
- API title
- API version
- First server URL, when present
- Every `GET` operation
- Each operation's `operationId`, method, and path
- A warning when a `GET` operation has no `operationId`
- Total number of discovered `GET` operations

## Non-goals

This slice does not implement:

- An MCP server or MCP transport
- Upstream HTTP requests
- Execution of any OpenAPI operation
- Authentication
- Remote OpenAPI URLs
- Docker packaging
- Configuration files
- Code generation
- Mutation operations such as `POST`, `PUT`, `PATCH`, or `DELETE`
- Policy, approval, evaluation, persistence, UI, or SaaS capabilities

## CLI contract

### Success

```bash
oasrelay inspect <local-spec-path>
```

Example output:

```text
OpenAPI: 3.0.3
API: Customer API
Version: 1.0.0
Server: http://localhost:8080

Available GET operations:

  getCustomer
    GET /customers/{customerId}

  listCustomers
    GET /customers

Total GET operations: 2
```

Operations are printed in deterministic path order so output and tests remain stable.

When a `GET` operation has no `operationId`, it remains visible:

```text
  <missing operationId>
    GET /health
    Warning: operationId is required for future MCP exposure
```

### Failure

The command writes a concise diagnostic to standard error and exits non-zero when:

- The command or required path argument is missing
- The file cannot be read
- The input is not valid YAML or JSON
- The document is not a valid OpenAPI document
- The OpenAPI version is not 3.x

Diagnostics must contain enough context to identify the file and failure without exposing stack traces during normal CLI use.

## Architecture

The initial implementation uses three small boundaries:

```text
cmd/oasrelay
    CLI argument handling and presentation
          |
          v
internal/openapi
    load, validate, and discover GET operations
          |
          v
OpenAPI parsing library
```

Proposed repository structure:

```text
cmd/
└── oasrelay/
    └── main.go
internal/
└── openapi/
    ├── inspect.go
    └── inspect_test.go
testdata/
├── customer-api.yaml
├── missing-operation-id.yaml
└── invalid-openapi.yaml
go.mod
README.md
LICENSE
```

The CLI layer does not traverse the OpenAPI document directly. The `internal/openapi` package returns a small project-owned inspection result, preventing presentation code from depending on the parser library's complete object graph.

## Internal data model

```go
type Inspection struct {
    OpenAPIVersion string
    Title          string
    APIVersion     string
    ServerURL      string
    Operations     []Operation
}

type Operation struct {
    OperationID string
    Method      string
    Path        string
}
```

A missing `operationId` is represented as an empty string. Presentation decides how to render the warning.

## Loading and validation

The inspector:

1. Accepts a filesystem path.
2. Reads only that local file.
3. Parses YAML or JSON based on content rather than filename alone.
4. Resolves document references supported by the selected OpenAPI parser.
5. Validates the resulting OpenAPI document.
6. Rejects non-3.x specifications.
7. Extracts only `GET` operations.
8. Sorts operations by path, then by operation ID for deterministic output.

External network access is not part of this slice. A local document that depends on unresolved remote references must fail clearly rather than fetching them implicitly.

## Error handling

Domain-facing errors are wrapped with operation context, for example:

```text
inspect ./openapi.yaml: read document: permission denied
inspect ./openapi.yaml: validate document: path /customers has an invalid parameter
inspect ./swagger.yaml: unsupported specification version "2.0"
```

The package returns errors; only `cmd/oasrelay` decides the exit code and writes to standard error.

## Testing

Tests cover the observable behavior at two levels.

### Package tests

- Valid YAML document
- Valid JSON document
- Invalid document
- Unsupported OpenAPI version
- Multiple paths returned in deterministic order
- Missing `operationId` preserved as an empty value
- First server URL extracted when present
- Empty server list handled without failure

### CLI-level behavior

The command entry point is structured so success and failure behavior can be exercised without calling `os.Exit` from testable logic. Tests verify:

- Missing arguments fail
- Valid input produces the expected report
- Invalid input writes an error and returns non-zero

## Definition of done

The slice is complete when these commands succeed:

```bash
go test ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

The output must match the fixture contents and the implementation must remain within the stated non-goals.

## Next slice

After Issue #1 is accepted, the next independent slice will select one `GET` operation by `operationId` and expose it as one stdio MCP tool. That work is not part of this design or implementation.

# OASRelay

OASRelay is a local-first runtime for exposing selected OpenAPI operations as MCP tools.

The current scope is intentionally narrow:

- inspect one local OpenAPI 3.x YAML or JSON document;
- select one parameterless `GET` operation by its exact `operationId`;
- expose that operation as exactly one MCP tool over stdio;
- execute one bounded upstream HTTP request when the tool is called.

## Requirements

- Go 1.25 or newer

## Build

```bash
go build -o oasrelay ./cmd/oasrelay
```

## Inspect an OpenAPI document

Run directly from the repository:

```bash
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

Or use the built binary:

```bash
./oasrelay inspect ./openapi.yaml
```

Example output:

```text
OpenAPI: 3.0.3
API: Customer API
Version: 1.0.0
Server: http://localhost:8080

Available GET operations:

  listCustomers
    GET /customers

  getCustomer
    GET /customers/{customerId}

Total GET operations: 2
```

The `inspect` command:

- reads only the supplied local file;
- accepts supported OpenAPI 3.x YAML and JSON documents;
- validates the document before reporting operations;
- resolves internal document references;
- rejects external file and URL references;
- lists only `GET` operations in deterministic path order;
- reports a warning when a `GET` operation has no `operationId`;
- returns exit code `1` for document errors and `2` for command usage errors.

## Serve one MCP tool

```bash
go run ./cmd/oasrelay serve \
  --operation-id listCustomers \
  ./openapi.yaml
```

With the built binary:

```bash
./oasrelay serve \
  --operation-id listCustomers \
  ./openapi.yaml
```

The process uses stdin and stdout for MCP protocol traffic. Configure the command and arguments in a stdio-capable MCP client; do not pipe human-readable output through stdout.

The selected OpenAPI operation must:

- have the exact requested `operationId`;
- use `GET`;
- have no path-level or operation-level parameters;
- have no request body;
- resolve to a static, absolute `http` or `https` server URL;
- use an MCP-compatible `operationId` containing 1–128 characters from `A-Z`, `a-z`, `0-9`, `_`, `-`, and `.`.

Server precedence is operation-level, then path-level, then document-level. The first server at the selected scope is used.

The tool accepts an empty object and returns structured output:

```json
{
  "status": 200,
  "contentType": "application/json",
  "body": "{\"items\":[]}"
}
```

Runtime limits are fixed in this slice:

- request timeout: 15 seconds;
- maximum response body: 1 MiB;
- transport: stdio only;
- exposed tools: exactly one.

A non-2xx HTTP response preserves `status`, `contentType`, and `body`, while marking the MCP tool result as an error. Network, timeout, cancellation, response-read, and oversized-body failures are returned as tool execution errors.

## Development

Run the verification commands:

```bash
go mod tidy
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

The MCP integration tests use the official Go SDK's in-memory transports and a real local `httptest` upstream server.

## Current scope boundary

Implemented:

- local OpenAPI 3.x inspection;
- YAML and JSON input;
- OpenAPI validation;
- internal reference resolution with external references blocked;
- deterministic `GET` operation discovery;
- exact selection of one supported `operationId`;
- one stdio MCP tool;
- one bounded upstream HTTP `GET` request;
- structured success and non-2xx output.

Not implemented:

- path, query, header, or cookie parameters;
- authentication or secret handling;
- request bodies or non-GET execution;
- multiple MCP tools;
- Streamable HTTP transport;
- remote OpenAPI loading;
- base URL overrides;
- Docker packaging;
- configuration or policy systems;
- code generation;
- persistence, UI, tenancy, billing, or SaaS features.

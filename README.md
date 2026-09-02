# OASRelay

OASRelay is a local-first project for exposing selected OpenAPI operations as MCP tools.

The current first slice does **not** run an MCP server. It validates one local OpenAPI 3.x YAML or JSON document and lists its `GET` operations.

## Requirements

- Go 1.25 or newer

## Inspect an OpenAPI document

Run directly from the repository:

```bash
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

Or build the CLI:

```bash
go build -o oasrelay ./cmd/oasrelay
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

## Current behavior

The `inspect` command:

- reads only the supplied local file;
- accepts supported OpenAPI 3.x YAML and JSON documents;
- validates the document before reporting operations;
- resolves internal document references;
- rejects external file and URL references;
- lists only `GET` operations in deterministic path order;
- reports a warning when a `GET` operation has no `operationId`;
- returns exit code `1` for document errors and `2` for command usage errors.

## Development

Run the verification commands:

```bash
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

## Current scope boundary

Implemented:

- local OpenAPI inspection;
- YAML and JSON input;
- OpenAPI validation;
- deterministic `GET` operation discovery;
- missing `operationId` reporting.

Not implemented:

- MCP server or transport;
- upstream API execution;
- authentication;
- Docker packaging;
- remote OpenAPI loading;
- configuration or policy systems;
- code generation;
- persistence, UI, or SaaS features.

The next independent slice will select one `GET` operation by `operationId` and expose it as one stdio MCP tool.

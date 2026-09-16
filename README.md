# OASRelay

OASRelay is a local-first runtime for exposing selected OpenAPI operations as MCP tools.

The current scope is intentionally narrow:

- inspect one local OpenAPI 3.x YAML or JSON document;
- select one `GET` operation by its exact `operationId`;
- support either no parameters or one required primitive operation-level query parameter;
- expose that operation as exactly one MCP tool over stdio;
- execute one bounded upstream HTTP request when the tool is called;
- run the same stdio runtime directly or in a minimal non-root Docker image.

## Requirements

- Go 1.25 or newer for source builds and development
- Docker for the container runtime and Docker acceptance test

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
- have no path-level parameters;
- have either no operation-level parameters or exactly one supported query parameter;
- have no request body;
- resolve to a static, absolute `http` or `https` server URL;
- use an MCP-compatible `operationId` containing 1–128 characters from `A-Z`, `a-z`, `0-9`, `_`, `-`, and `.`.

Server precedence is operation-level, then path-level, then document-level. The first server at the selected scope is used.

### One required primitive query parameter

A supported parameter must be operation-level, `in: query`, and `required: true`. Its schema must be a plain primitive `string`, `integer`, `number`, or `boolean` without additional constraints or custom serialization.

For example:

```yaml
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
      responses:
        "200":
          description: Customer collection
```

The MCP tool exposes an input schema equivalent to:

```json
{
  "type": "object",
  "properties": {
    "limit": {"type": "integer"}
  },
  "required": ["limit"],
  "additionalProperties": false
}
```

Calling it with:

```json
{"limit": 25}
```

issues:

```text
GET /customers?limit=25
```

Missing required input, explicit `null`, a wrong primitive type, or an unknown input field is returned as an MCP tool error before any upstream request is sent. Query values use standard URL query escaping. Integer values are emitted in canonical base-10 form even when a mathematically integral JSON number arrives as `25.0` or `25e0`.

If the selected server URL already contains a raw query string, OASRelay preserves that existing query byte-for-byte and appends only the encoded operation parameter. This avoids silently dropping RFC-valid query pairs that Go's form-style query parser does not accept.

A parameterless operation continues to expose an empty object input schema.

The tool response remains:

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

## Run with Docker

Build the local image:

```bash
docker build -t oasrelay:local .
```

Mount the OpenAPI document read-only and keep stdin open for MCP protocol traffic:

```bash
docker run --rm -i \
  --mount type=bind,src=/absolute/path/openapi.yaml,dst=/work/openapi.yaml,readonly \
  oasrelay:local \
  serve --operation-id listCustomers /work/openapi.yaml
```

Use an absolute host path for the bind mount. A stdio-capable MCP client should launch this command directly; stdout is reserved for newline-delimited MCP JSON messages and operational errors are written to stderr.

The image:

- uses a multi-stage Go 1.25 build;
- produces a statically linked Linux binary with `CGO_ENABLED=0`;
- uses `scratch` as the final image;
- runs as numeric non-root user `65532:65532`;
- includes CA certificates for HTTPS upstream APIs;
- sets `/oasrelay` as the entrypoint and `/work` as the working directory;
- contains no bundled OpenAPI documents, credentials, shell, or package manager.

## Development

Run the standard verification commands:

```bash
go mod tidy
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

Run the Docker build and end-to-end stdio acceptance test:

```bash
./scripts/test-docker.sh
```

The in-process MCP tests use the official Go SDK's in-memory transports. The Docker acceptance test uses the SDK's `CommandTransport`, launches `docker run -i`, mounts a generated spec read-only, and calls a real host-side HTTP fixture through the container.

## Current scope boundary

Implemented:

- local OpenAPI 3.x inspection;
- YAML and JSON input;
- OpenAPI validation;
- internal reference resolution with external references blocked;
- deterministic `GET` operation discovery;
- exact selection of one supported `operationId`;
- zero parameters or one required operation-level primitive query parameter;
- dynamic MCP input schema for that query parameter;
- query argument validation and URL binding;
- canonical integer query serialization;
- preservation of existing raw server URL queries when appending the operation parameter;
- one stdio MCP tool;
- one bounded upstream HTTP `GET` request;
- structured success and non-2xx output;
- minimal non-root Docker packaging;
- container-level MCP stdio acceptance testing.

Not implemented:

- optional or multiple query parameters;
- path, header, or cookie parameters;
- path-level parameter inheritance;
- arrays, objects, enums, unions, nullable schemas, schema constraints, or custom parameter serialization;
- authentication or secret handling;
- request bodies or non-GET execution;
- multiple MCP tools;
- Streamable HTTP transport;
- remote OpenAPI loading;
- base URL overrides;
- Docker Compose or Kubernetes manifests;
- published registry images or release automation;
- configuration or policy systems;
- code generation;
- persistence, UI, tenancy, billing, or SaaS features.

# OASRelay

OASRelay is a local-first runtime for exposing selected OpenAPI operations as MCP tools.

The current scope is intentionally narrow:

- inspect one local OpenAPI 3.x YAML or JSON document;
- select one `GET` operation by its exact `operationId`;
- support no parameters, one or two primitive query parameters (each required or optional), one or two required primitive path parameters, or exactly one path plus one query parameter, with at most two effective parameters total after path-item inheritance and operation overrides;
- expose that operation as exactly one MCP tool over stdio;
- execute one bounded upstream HTTP request when the tool is called;
- optionally attach one Bearer token from process environment to upstream requests;
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
- have no effective parameters, one or two supported query parameters, one or two supported path parameters, or exactly one supported path plus one supported query parameter, with at most two parameters total after inheritance and overrides;
- have no request body;
- resolve to a static, absolute `http` or `https` server URL;
- use an MCP-compatible `operationId` containing 1–128 characters from `A-Z`, `a-z`, `0-9`, `_`, `-`, and `.`.

Server precedence is operation-level, then path-level, then document-level. The first server at the selected scope is used.

### Path-item parameter inheritance

Parameters may be declared on the path item, the operation, or both. Path-item parameters are inherited by the selected operation. An operation-level parameter with the same exact `(name, in)` identity replaces the complete inherited definition, including its type and requiredness. Other inherited parameters remain present; an empty operation parameter list does not remove them. These override rules follow the [OpenAPI parameter contract](https://spec.openapis.org/oas/v3.0.3.html#path-item-object).

For example:

```yaml
paths:
  /customers/{customerId}/orders:
    parameters:
      - name: customerId
        in: path
        required: true
        schema:
          type: string
      - name: limit
        in: query
        required: true
        schema:
          type: string
    get:
      operationId: getCustomerOrders
      parameters:
        - name: limit
          in: query
          required: false
          schema:
            type: integer
      responses:
        "200":
          description: Customer orders
```

The effective MCP input contains a required `customerId` string and an optional `limit` integer. `{"customerId":"cus_123"}` omits the query pair; adding `"limit":25` sends `?limit=25`. The complete runnable fixture is [testdata/inherited-parameters.yaml](testdata/inherited-parameters.yaml).

The two-parameter cap applies after merging, so the example's three source declarations produce two effective parameters. Effective order starts with path-item declaration order, overrides keep their inherited slots, and operation-only parameters append in declaration order. Query serialization follows this effective order. This deterministic ordering is OASRelay policy; OpenAPI does not prescribe query pair order.

Duplicate `(name, in)` entries within either source remain invalid. Distinct path/query parameters sharing a name remain unsupported because MCP input names must be unique. All existing primitive type, required-path, placeholder, serialization, and pre-network argument checks also apply to inherited parameters. Internal parameter references are supported; external references remain blocked.

### One primitive query parameter

A supported query parameter must use `in: query` and may be declared on the path item or operation. It may be required or optional. Its effective schema must be a plain primitive `string`, `integer`, `number`, or `boolean` without additional constraints or custom serialization.

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

Query values use standard URL query escaping. Integer values are emitted in canonical base-10 form even when a mathematically integral JSON number arrives as `25.0` or `25e0`.

An optional query parameter is represented by `required: false` or by omitting the OpenAPI `required` field. For example:

```yaml
paths:
  /customers:
    get:
      operationId: listCustomers
      parameters:
        - name: limit
          in: query
          required: false
          schema:
            type: integer
      responses:
        "200":
          description: Customer collection
```

The MCP input schema still exposes `limit` as an integer property, but it does not list it under `required`.

Calling the tool with:

```json
{}
```

sends no operation-level `limit` query pair. Calling it with:

```json
{"limit": 25}
```

adds:

```text
?limit=25
```

Omission is not the same as `null`: `{"limit": null}` remains invalid because nullable schemas are unsupported. OpenAPI schema defaults also remain unsupported; OASRelay does not inject a default query value when the caller omits an optional argument.

If the selected server URL already contains a raw query string, OASRelay preserves that existing query byte-for-byte. When the optional query argument is omitted, the existing query is left unchanged; when supplied, only the encoded operation parameter is appended. This avoids silently dropping RFC-valid query pairs that Go's form-style query parser does not accept.

### Two primitive query parameters

An operation without a path parameter may have exactly two effective primitive query parameters. Each may independently be required or optional; the same plain primitive schemas and default query serialization rules apply to both.

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
        - name: cursor
          in: query
          required: false
          schema:
            type: string
      responses:
        "200":
          description: Customer collection
```

Calling with `{"limit": 25}` sends `GET /customers?limit=25`. Calling with `{"limit": 25, "cursor": "next page"}` sends `GET /customers?limit=25&cursor=next+page`.

Query pairs follow effective parameter order after inheritance and overrides, regardless of the order of fields in the MCP input. With operation-only parameters, this is their declaration order. Existing raw server query text is preserved, and supplied query pairs are appended.

If both query parameters are optional, `{}` is valid and adds no operation query pairs. Omission remains distinct from an explicit empty string, which is sent as an empty query value. Explicit `null`, missing required values, wrong primitive types, and unknown fields fail before any upstream request. Schema defaults remain unsupported and are never injected.

The total effective parameter cap remains two. One path plus two query parameters and three query parameters remain unsupported.

### One required primitive path parameter

A supported path parameter may be declared on the path item or operation, must use `in: path`, be `required: true`, and use the default simple path serialization. Its name must match exactly one `{name}` placeholder in the operation path, and its effective schema must be a plain primitive `string`, `integer`, `number`, or `boolean` without additional constraints.

For example:

```yaml
paths:
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
```

The MCP tool exposes one required `customerId` string property. Calling it with:

```json
{"customerId": "cus_123"}
```

issues:

```text
GET /customers/cus_123
```

A path argument is encoded as one path segment. For example, `a/b` is sent as `a%2Fb`, so caller data cannot introduce an extra path segment. Literal `.` and `..` values are percent-encoded as data instead of being left as dot-segments. Existing server URL paths and raw query strings are preserved.

### Two required primitive path parameters

An operation without query parameters may have exactly two effective required primitive path parameters. Both use the same plain primitive schemas, default simple serialization, and single-segment escaping rules as a one-path operation. Each name must be distinct and match exactly one placeholder; undeclared or repeated placeholders are rejected.

For example:

```yaml
paths:
  /customers/{customerId}/orders/{orderId}:
    get:
      operationId: getCustomerOrder
      parameters:
        - name: orderId
          in: path
          required: true
          schema:
            type: string
        - name: customerId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Customer order
```

Calling with `{"customerId": "cus_123", "orderId": "42"}` sends `GET /customers/cus_123/orders/42`. Declaration order may differ from route order; binding uses each parameter's name. Both fields are required in the closed MCP input object.

Caller values are data even when they resemble placeholders. For example, `{"customerId": "{orderId}", "orderId": "a/b"}` sends `GET /customers/%7BorderId%7D/orders/a%2Fb`. Substitution scans the original template once and never interprets an inserted argument as another placeholder. Existing escaped server paths and raw queries are preserved.

Missing values, explicit `null`, wrong primitive types, or unknown fields fail before an upstream request. Two path parameters plus any query, three path parameters, optional paths, and complex schemas remain unsupported.

### One required path + one query parameter

OASRelay also supports exactly one effective required primitive path parameter together with exactly one effective primitive query parameter. The query parameter may be required or optional; the path parameter remains required.

For example:

```yaml
paths:
  /customers/{customerId}/orders:
    get:
      operationId: getCustomerOrders
      parameters:
        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: limit
          in: query
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: Customer orders
```

When the query parameter is required, the MCP input is a closed object with both properties required:

```json
{
  "customerId": "cus_123",
  "limit": 25
}
```

The upstream request is:

```text
GET /customers/cus_123/orders?limit=25
```

If the query parameter is optional, `customerId` remains required while the query property is omitted from the MCP `required` list. A call containing only `{"customerId": "cus_123"}` sends `GET /customers/cus_123/orders`; supplying `limit` appends the query pair.

OpenAPI declaration order does not matter. Binding is always path first and query second when the query value is supplied. Existing path escaping, raw-query preservation, primitive type validation, canonical integer serialization, and Bearer security rules remain unchanged.

The total effective parameter cap remains two. One path plus two query parameters, two paths plus any query, more than two effective parameters, optional path parameters, headers, cookies, and custom serialization remain unsupported.

For both query and path parameters, missing required input, explicit `null`, a wrong primitive type, or an unknown input field is returned as an MCP tool error before any upstream request is sent. A parameterless operation continues to expose an empty object input schema.

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

## Optional Bearer authentication

Set `OASRELAY_BEARER_TOKEN` when the upstream API requires a Bearer token:

```bash
OASRELAY_BEARER_TOKEN='<token>' \
  ./oasrelay serve --operation-id listCustomers ./openapi.yaml
```

When the variable contains a non-empty token, OASRelay sends:

```text
Authorization: Bearer <token>
```

Bearer authentication requires an `https://` upstream. If a non-empty token is configured for an `http://` endpoint, OASRelay rejects the request before any network call is made. Unauthenticated HTTP endpoints remain supported.

The token is runtime process configuration. It is not an MCP tool argument, is not added to `tools/list`, and is not included in tool output. An unset, empty, or whitespace-only value keeps the existing unauthenticated behavior. Values containing carriage-return or line-feed characters are rejected before the MCP stdio runtime starts, and the rejection message does not echo the secret value.

Bearer-authenticated redirects are followed only when the redirect remains on the exact same origin (scheme, host, and port). A cross-origin redirect is returned as the upstream 3xx response instead of being followed, preventing the runtime Bearer token from being forwarded to another origin.

This is deliberately not OpenAPI security processing yet. OASRelay does not currently inspect `securitySchemes`, choose credentials from a specification, refresh OAuth tokens, or support API-key/Basic/custom-header authentication.

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

For an authenticated HTTPS upstream, pass the token only at container runtime:

```bash
docker run --rm -i \
  -e OASRELAY_BEARER_TOKEN \
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

The in-process MCP tests use the official Go SDK's in-memory transports. The Docker acceptance test uses the SDK's `CommandTransport`, launches `docker run -i`, mounts a generated spec and ephemeral test CA read-only, forwards a test Bearer token through the container environment, and calls a real host-side HTTPS fixture through the container.

## Current scope boundary

Implemented:

- local OpenAPI 3.x inspection;
- YAML and JSON input;
- OpenAPI validation;
- internal reference resolution with external references blocked;
- deterministic `GET` operation discovery;
- exact selection of one supported `operationId`;
- zero parameters, one or two effective primitive query parameters (each required or optional), one or two effective required primitive path parameters, or exactly one path plus one query parameter, with at most two parameters total;
- path-item parameter inheritance with complete operation-level overrides by `(name, in)` and deterministic effective ordering;
- dynamic closed MCP input schema that preserves required versus optional query semantics;
- query argument validation and URL binding;
- two-query schema and binding with independent requiredness, declaration-order serialization, and optional omission;
- safe single-segment path argument validation and binding;
- two required path parameters with name-based, non-recursive template substitution;
- combined path-then-query validation and binding for exactly one parameter in each location, with the query required or optional;
- canonical integer serialization;
- preservation of existing server URL paths and raw queries;
- optional process-level Bearer authentication through `OASRELAY_BEARER_TOKEN`, restricted to HTTPS upstreams;
- one stdio MCP tool;
- one bounded upstream HTTP `GET` request;
- structured success and non-2xx output;
- minimal non-root Docker packaging;
- container-level MCP stdio acceptance for required-path plus optional-query, required-query plus optional-query, two required paths, and inherited-path plus overridden optional-query, with Bearer forwarding.

Not implemented:

- arbitrary multiple operation parameters, including three query parameters, three path parameters, one path plus two query parameters, or two paths plus a query;
- optional path parameters;
- header or cookie parameters;
- arrays, objects, enums, unions, nullable schemas, schema constraints, schema defaults, or custom parameter serialization;
- OpenAPI `securitySchemes` processing;
- OAuth/OIDC, API-key, Basic, arbitrary-header, token-refresh, or secret-store authentication;
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

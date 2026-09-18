# Combined Path + Query Parameter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing one-tool GET runtime so one selected OpenAPI operation may expose exactly one required primitive operation-level path parameter together with exactly one required primitive operation-level query parameter.

**Architecture:** Keep the current project-owned `SelectedOperation` model with separate `PathParameter` and `QueryParameter` pointers; both non-nil becomes the new valid combined shape. The selector classifies at most two operation-level parameters by location, while the MCP layer composes the existing single-path and single-query schema/binding rules in deterministic path-then-query order. Do not introduce a generic parameter IR, serialization engine, or arbitrary multi-parameter collection.

**Tech Stack:** Go 1.25+, Go standard library, `github.com/getkin/kin-openapi` v0.149.0, official `github.com/modelcontextprotocol/go-sdk/mcp`, existing Docker acceptance infrastructure.

**Spec:** GitHub Issue #16 — https://github.com/kefyusuf/oasrelay/issues/16

## Global Constraints

- GET only.
- Exactly one MCP tool remains exposed.
- Preserve existing supported shapes: zero parameters, one required primitive query parameter, and one required primitive path parameter.
- Add exactly one new supported shape: one required primitive path parameter plus one required primitive query parameter.
- Both parameters must be declared directly on the operation; path-item-level parameters remain rejected.
- Parameter declaration order in OpenAPI must not affect behavior.
- Supported schema types remain exactly `string`, `integer`, `number`, and `boolean`.
- Existing plain-primitive restrictions remain unchanged: no arrays, objects, enums, unions, nullable schemas, formats, numeric/string constraints, conditional schemas, defaults, or custom serialization.
- Path serialization remains default `simple` with `explode=false`.
- Query serialization remains default `form` with `explode=true` and `allowReserved=false`.
- A path and query parameter may not share the same name.
- More than two operation-level parameters remain rejected.
- Two query parameters remain rejected.
- Two path parameters remain rejected.
- Header, cookie, and optional parameters remain rejected.
- MCP input remains a closed object with `additionalProperties: false`.
- Combined input properties are both required.
- Binding order is path first, query second.
- Existing raw server query text must remain byte-for-byte intact before the encoded operation query parameter is appended.
- Existing path segment escaping, dot-segment hardening, and canonical integer serialization remain unchanged.
- Existing Bearer behavior remains unchanged: credentials are process-only, require HTTPS, and do not follow cross-origin redirects.
- Do not add dependencies.
- Do not add generic `[]Parameter`, `ParameterIR`, policy, configuration, security-scheme parsing, multiple tools, non-GET execution, or Streamable HTTP.

## File Map

- Modify: `internal/openapi/select.go` — classify zero, single, or combined supported operation-level parameters.
- Modify: `internal/openapi/query_parameter_test.go` — keep the two-query rejection but update the expected bounded-scope error.
- Modify: `internal/openapi/path_parameter_test.go` — remove the obsolete path+query rejection expectation.
- Create: `internal/openapi/combined_parameter_test.go` — selector acceptance/rejection contract for the combined shape.
- Modify: `internal/mcpserver/server.go` — allow both parameter pointers, generate two-property input schema, and compose path/query binding.
- Create: `internal/mcpserver/combined_parameter_test.go` — MCP schema, exact URL binding, and pre-upstream validation tests.
- Create: `cmd/oasrelay/combined_parameter_test.go` — CLI selection integration for the new shape; no production CLI change is expected.
- Modify: `internal/containertest/docker_stdio_test.go` — strengthen the existing TLS/Bearer container acceptance to call a combined path+query tool.
- Modify: `README.md` — document the fourth supported operation shape and keep non-goals explicit.

---

### Task 1: Select Exactly One Path + One Query Parameter

**Files:**
- Create: `internal/openapi/combined_parameter_test.go`
- Modify: `internal/openapi/select.go`
- Modify: `internal/openapi/query_parameter_test.go`
- Modify: `internal/openapi/path_parameter_test.go`
- Create: `cmd/oasrelay/combined_parameter_test.go`

**Interfaces:**
- Consumes: existing `SelectGET(path, operationID string) (SelectedOperation, error)`, `QueryParameter`, `PathParameter`, and `writeSelectionSpec`.
- Produces: the same public `SelectedOperation` shape, now permitting both `QueryParameter` and `PathParameter` to be non-nil when exactly one supported parameter exists in each location.
- Produces internal helper:
  ```go
  func supportedOperationParameters(
      operationID, route string,
      parameters openapi3.Parameters,
  ) (*QueryParameter, *PathParameter, error)
  ```

- [ ] **Step 1: Add a selector RED test for both OpenAPI declaration orders**

Create `internal/openapi/combined_parameter_test.go` with the following acceptance test:

```go
package openapi

import (
    "fmt"
    "strings"
    "testing"
)

func TestSelectGETAcceptsOnePathAndOneQueryParameterInEitherOrder(t *testing.T) {
    tests := []struct {
        name       string
        parameters string
    }{
        {
            name: "path then query",
            parameters: `        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: limit
          in: query
          required: true
          schema:
            type: integer`,
        },
        {
            name: "query then path",
            parameters: `        - name: limit
          in: query
          required: true
          schema:
            type: integer
        - name: customerId
          in: path
          required: true
          schema:
            type: string`,
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.0.3
info:
  title: Combined API
  version: 1.0.0
servers:
  - url: https://example.test/api?token=a;b
paths:
  /customers/{customerId}/orders:
    get:
      operationId: getCustomerOrders
      parameters:
%s
      responses:
        "200":
          description: Customer orders
`, test.parameters))

            got, err := SelectGET(path, "getCustomerOrders")
            if err != nil {
                t.Fatalf("SelectGET() error = %v", err)
            }
            if got.PathParameter == nil ||
                got.PathParameter.Name != "customerId" ||
                got.PathParameter.Type != "string" {
                t.Fatalf("PathParameter = %#v", got.PathParameter)
            }
            if got.QueryParameter == nil ||
                got.QueryParameter.Name != "limit" ||
                got.QueryParameter.Type != "integer" {
                t.Fatalf("QueryParameter = %#v", got.QueryParameter)
            }
            if got.Endpoint != "https://example.test/api/customers/%7BcustomerId%7D/orders?token=a;b" {
                t.Fatalf("Endpoint = %q", got.Endpoint)
            }
        })
    }
}
```

- [ ] **Step 2: Add selector RED tests for the newly explicit rejection boundaries**

Append this table-driven test to the same file:

```go
func TestSelectGETRejectsUnsupportedCombinedParameterShapes(t *testing.T) {
    tests := []struct {
        name       string
        route      string
        parameters string
        message    string
    }{
        {
            name:  "two query parameters",
            route: "/customers",
            parameters: `        - name: limit
          in: query
          required: true
          schema:
            type: integer
        - name: cursor
          in: query
          required: true
          schema:
            type: string`,
            message: "at most one query parameter",
        },
        {
            name:  "two path parameters",
            route: "/customers/{customerId}/orders/{orderId}",
            parameters: `        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: orderId
          in: path
          required: true
          schema:
            type: string`,
            message: "at most one path parameter",
        },
        {
            name:  "three operation parameters",
            route: "/customers/{customerId}",
            parameters: `        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: limit
          in: query
          required: true
          schema:
            type: integer
        - name: cursor
          in: query
          required: true
          schema:
            type: string`,
            message: "supports at most two operation-level parameters",
        },
        {
            name:  "same MCP argument name across locations",
            route: "/customers/{id}",
            parameters: `        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: id
          in: query
          required: true
          schema:
            type: string`,
            message: "share parameter name",
        },
        {
            name:  "header with path",
            route: "/customers/{customerId}",
            parameters: `        - name: customerId
          in: path
          required: true
          schema:
            type: string
        - name: X-Limit
          in: header
          required: true
          schema:
            type: integer`,
            message: "supports query and path parameters only",
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            path := writeSelectionSpec(t, fmt.Sprintf(`openapi: 3.0.3
info:
  title: Combined API
  version: 1.0.0
servers:
  - url: https://example.test
paths:
  %s:
    get:
      operationId: combinedOperation
      parameters:
%s
      responses:
        "200":
          description: Combined operation
`, test.route, test.parameters))

            _, err := SelectGET(path, "combinedOperation")
            if err == nil || !strings.Contains(err.Error(), test.message) {
                t.Fatalf("error = %v, want substring %q", err, test.message)
            }
        })
    }
}
```

- [ ] **Step 3: Replace the obsolete CLI/path rejection expectation with a CLI RED integration test**

Delete `TestSelectGETStillRejectsQueryAndPathCombination` from `internal/openapi/path_parameter_test.go`.

Create `cmd/oasrelay/combined_parameter_test.go`:

```go
package main

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "testing"

    oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func TestRunServeAcceptsOnePathAndOneQueryParameter(t *testing.T) {
    path := filepath.Join(t.TempDir(), "openapi.yaml")
    document := `openapi: 3.0.3
info:
  title: Combined API
  version: 1.0.0
servers:
  - url: https://example.test/api
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
`
    if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
        t.Fatalf("write OpenAPI fixture: %v", err)
    }

    var stdout bytes.Buffer
    var stderr bytes.Buffer
    var selected oasopenapi.SelectedOperation
    calls := 0

    code := runWithServe(
        []string{"serve", "--operation-id", "getCustomerOrders", path},
        &stdout,
        &stderr,
        func(_ context.Context, operation oasopenapi.SelectedOperation) error {
            calls++
            selected = operation
            return nil
        },
    )

    if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 || calls != 1 {
        t.Fatalf(
            "code = %d; stdout = %q; stderr = %q; calls = %d",
            code,
            stdout.String(),
            stderr.String(),
            calls,
        )
    }
    if selected.PathParameter == nil || selected.PathParameter.Name != "customerId" {
        t.Fatalf("selected PathParameter = %#v", selected.PathParameter)
    }
    if selected.QueryParameter == nil || selected.QueryParameter.Name != "limit" {
        t.Fatalf("selected QueryParameter = %#v", selected.QueryParameter)
    }
}
```

- [ ] **Step 4: Update the existing query-parameter regression messages**

In `internal/openapi/query_parameter_test.go`, keep the existing `multiple parameters` case as two query parameters but change its expected message from:

```go
message: "supports at most one operation-level parameter",
```

to:

```go
message: "at most one query parameter",
```

The location classifier now rejects headers before `supportedQueryParameter` runs, so also change the existing `header parameter` case from:

```go
message: "supports query parameters only",
```

to:

```go
message: "supports query and path parameters only",
```

- [ ] **Step 5: Run the selector and CLI tests and verify RED**

Run:

```bash
go test ./internal/openapi ./cmd/oasrelay
```

Expected: FAIL because `SelectGET` still rejects any operation containing more than one operation-level parameter.

- [ ] **Step 6: Replace single-parameter classification with bounded two-location classification**

In `internal/openapi/select.go`, change the count guard inside `selectGETOperation` to:

```go
if len(operation.Parameters) > 2 {
    return SelectedOperation{}, fmt.Errorf(
        "operationId %q has %d operation parameters; this version supports at most two operation-level parameters",
        operationID,
        len(operation.Parameters),
    )
}
```

Replace the call to `supportedOperationParameter` with:

```go
queryParameter, pathParameter, err := supportedOperationParameters(
    operationID,
    route,
    operation.Parameters,
)
if err != nil {
    return SelectedOperation{}, err
}
return buildSelectedOperation(
    document,
    route,
    method,
    item,
    operation,
    queryParameter,
    pathParameter,
)
```

Replace `supportedOperationParameter` with:

```go
func supportedOperationParameters(
    operationID, route string,
    parameters openapi3.Parameters,
) (*QueryParameter, *PathParameter, error) {
    var queryRaw *openapi3.Parameter
    var pathRaw *openapi3.Parameter

    for _, parameterRef := range parameters {
        if parameterRef == nil || parameterRef.Value == nil {
            return nil, nil, fmt.Errorf(
                "operationId %q has an unresolved parameter; this version requires resolved operation parameters",
                operationID,
            )
        }

        parameter := parameterRef.Value
        switch parameter.In {
        case openapi3.ParameterInQuery:
            if queryRaw != nil {
                return nil, nil, fmt.Errorf(
                    "operationId %q supports at most one query parameter",
                    operationID,
                )
            }
            queryRaw = parameter

        case openapi3.ParameterInPath:
            if pathRaw != nil {
                return nil, nil, fmt.Errorf(
                    "operationId %q supports at most one path parameter",
                    operationID,
                )
            }
            pathRaw = parameter

        default:
            return nil, nil, fmt.Errorf(
                "operationId %q parameter %q is in %s; this version supports query and path parameters only",
                operationID,
                parameter.Name,
                parameter.In,
            )
        }
    }

    if queryRaw != nil && pathRaw != nil && queryRaw.Name == pathRaw.Name {
        return nil, nil, fmt.Errorf(
            "operationId %q path and query parameters share parameter name %q; MCP input names must be unique",
            operationID,
            queryRaw.Name,
        )
    }

    var queryParameter *QueryParameter
    if queryRaw != nil {
        parameter, err := supportedQueryParameter(operationID, queryRaw)
        if err != nil {
            return nil, nil, err
        }
        queryParameter = parameter
    }

    var pathParameter *PathParameter
    if pathRaw != nil {
        parameter, err := supportedPathParameter(operationID, route, pathRaw)
        if err != nil {
            return nil, nil, err
        }
        pathParameter = parameter
    }

    return queryParameter, pathParameter, nil
}
```

Change `supportedQueryParameter` to consume one already-resolved parameter:

```go
func supportedQueryParameter(
    operationID string,
    parameter *openapi3.Parameter,
) (*QueryParameter, error) {
    if parameter == nil {
        return nil, fmt.Errorf(
            "operationId %q has an unresolved query parameter",
            operationID,
        )
    }
    if parameter.In != openapi3.ParameterInQuery {
        return nil, fmt.Errorf(
            "operationId %q parameter %q is in %s; this version supports query parameters only",
            operationID,
            parameter.Name,
            parameter.In,
        )
    }
    if !parameter.Required {
        return nil, fmt.Errorf(
            "operationId %q query parameter %q must be required",
            operationID,
            parameter.Name,
        )
    }

    serialization, err := parameter.SerializationMethod()
    if err != nil {
        return nil, fmt.Errorf(
            "operationId %q query parameter %q: %w",
            operationID,
            parameter.Name,
            err,
        )
    }
    if serialization.Style != "form" ||
        !serialization.Explode ||
        parameter.AllowReserved {
        return nil, fmt.Errorf(
            "operationId %q query parameter %q does not use default query serialization",
            operationID,
            parameter.Name,
        )
    }

    if parameter.Schema == nil ||
        parameter.Schema.Value == nil ||
        !isPlainPrimitiveSchema(parameter.Schema.Value) {
        return nil, fmt.Errorf(
            "operationId %q query parameter %q must use a plain primitive schema",
            operationID,
            parameter.Name,
        )
    }

    return &QueryParameter{
        Name: parameter.Name,
        Type: (*parameter.Schema.Value.Type)[0],
    }, nil
}
```

Do not change `QueryParameter`, `PathParameter`, or `SelectedOperation`.

- [ ] **Step 7: Run selector and CLI tests and verify GREEN**

Run:

```bash
gofmt -w internal/openapi/select.go internal/openapi/combined_parameter_test.go internal/openapi/query_parameter_test.go internal/openapi/path_parameter_test.go cmd/oasrelay/combined_parameter_test.go
go test ./internal/openapi ./cmd/oasrelay
```

Expected: PASS.

- [ ] **Step 8: Commit the selector slice**

```bash
git add internal/openapi/select.go   internal/openapi/combined_parameter_test.go   internal/openapi/query_parameter_test.go   internal/openapi/path_parameter_test.go   cmd/oasrelay/combined_parameter_test.go
git commit -m "feat: select combined path and query parameters"
```

---

### Task 2: Expose a Two-Property MCP Input Schema

**Files:**
- Create: `internal/mcpserver/combined_parameter_test.go`
- Modify: `internal/mcpserver/server.go`

**Interfaces:**
- Consumes: `SelectedOperation.PathParameter` and `SelectedOperation.QueryParameter`.
- Produces: one closed MCP object schema whose `required` order is deterministic: path parameter first, query parameter second.
- Preserves: parameterless and existing single-parameter schemas.

- [ ] **Step 1: Write the MCP schema RED test**

Create `internal/mcpserver/combined_parameter_test.go`:

```go
package mcpserver

import (
    "context"
    "encoding/json"
    "net/http"
    "testing"

    oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func combinedOperation(endpoint string) oasopenapi.SelectedOperation {
    return oasopenapi.SelectedOperation{
        OperationID: "getCustomerOrders",
        Method:      http.MethodGet,
        Path:        "/customers/{customerId}/orders",
        Endpoint:    endpoint,
        PathParameter: &oasopenapi.PathParameter{
            Name: "customerId",
            Type: "string",
        },
        QueryParameter: &oasopenapi.QueryParameter{
            Name: "limit",
            Type: "integer",
        },
    }
}

func TestServerExposesCombinedPathAndQueryInputSchema(t *testing.T) {
    server, err := New(
        combinedOperation("http://example.test/customers/%7BcustomerId%7D/orders"),
        http.DefaultClient,
    )
    if err != nil {
        t.Fatalf("New() error = %v", err)
    }

    session, cleanup := connectClient(t, server)
    defer cleanup()

    listed, err := session.ListTools(context.Background(), nil)
    if err != nil {
        t.Fatalf("ListTools() error = %v", err)
    }
    if len(listed.Tools) != 1 {
        t.Fatalf("len(Tools) = %d, want 1", len(listed.Tools))
    }

    encoded, err := json.Marshal(listed.Tools[0].InputSchema)
    if err != nil {
        t.Fatalf("marshal input schema: %v", err)
    }
    var schema map[string]any
    if err := json.Unmarshal(encoded, &schema); err != nil {
        t.Fatalf("unmarshal input schema: %v", err)
    }

    if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
        t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
    }

    required, ok := schema["required"].([]any)
    if !ok ||
        len(required) != 2 ||
        required[0] != "customerId" ||
        required[1] != "limit" {
        t.Fatalf("required = %#v, want [customerId limit]", schema["required"])
    }

    properties, ok := schema["properties"].(map[string]any)
    if !ok {
        t.Fatalf("properties = %#v", schema["properties"])
    }
    customerID, ok := properties["customerId"].(map[string]any)
    if !ok || customerID["type"] != "string" {
        t.Fatalf("customerId schema = %#v", properties["customerId"])
    }
    limit, ok := properties["limit"].(map[string]any)
    if !ok || limit["type"] != "integer" {
        t.Fatalf("limit schema = %#v", properties["limit"])
    }
}
```

- [ ] **Step 2: Run the MCP package test and verify RED**

Run:

```bash
go test ./internal/mcpserver -run TestServerExposesCombinedPathAndQueryInputSchema -count=1
```

Expected: FAIL because `New` still rejects simultaneous query/path parameters.

- [ ] **Step 3: Replace the blanket combined-parameter rejection with a defensive duplicate-name check**

In `internal/mcpserver/server.go`, replace:

```go
if operation.QueryParameter != nil && operation.PathParameter != nil {
    return nil, fmt.Errorf("operation cannot expose query and path parameters together")
}
```

with:

```go
if operation.QueryParameter != nil &&
    operation.PathParameter != nil &&
    operation.QueryParameter.Name == operation.PathParameter.Name {
    return nil, fmt.Errorf(
        "path and query parameters share MCP input name %q",
        operation.QueryParameter.Name,
    )
}
```

The selector is the primary contract gate; this remains a defensive invariant for direct `SelectedOperation` callers.

- [ ] **Step 4: Replace one-property schema construction with deterministic composition**

Replace `toolInputSchema` and remove `operationInputParameter`. Use:

```go
func toolInputSchema(operation oasopenapi.SelectedOperation) map[string]any {
    properties := map[string]any{}
    required := []string{}

    if parameter := operation.PathParameter; parameter != nil {
        properties[parameter.Name] = map[string]any{
            "type": parameter.Type,
        }
        required = append(required, parameter.Name)
    }

    if parameter := operation.QueryParameter; parameter != nil {
        properties[parameter.Name] = map[string]any{
            "type": parameter.Type,
        }
        required = append(required, parameter.Name)
    }

    schema := map[string]any{
        "type":                 "object",
        "properties":           properties,
        "additionalProperties": false,
    }
    if len(required) != 0 {
        schema["required"] = required
    }
    return schema
}
```

Do not add an internal generic parameter slice.

- [ ] **Step 5: Run all MCP schema regressions and verify GREEN**

Run:

```bash
gofmt -w internal/mcpserver/server.go internal/mcpserver/combined_parameter_test.go
go test ./internal/mcpserver -count=1
```

Expected: the new two-property schema test and existing parameterless/single-query/single-path tests all PASS.

- [ ] **Step 6: Commit the schema slice**

```bash
git add internal/mcpserver/server.go internal/mcpserver/combined_parameter_test.go
git commit -m "feat: expose combined parameter schema"
```

---

### Task 3: Bind Path First, Then Query

**Files:**
- Modify: `internal/mcpserver/combined_parameter_test.go`
- Modify: `internal/mcpserver/server.go`

**Interfaces:**
- Consumes: existing `bindPathParameter`, `bindQueryParameter`, `primitiveQueryValue`, and `escapePathSegment`.
- Produces internal helper:
  ```go
  func bindPathAndQueryParameters(
      operation oasopenapi.SelectedOperation,
      input map[string]json.RawMessage,
  ) (string, error)
  ```
- Binding invariant: validate the exact two argument names, bind the path into the endpoint first, then append the query parameter to that resulting endpoint.

- [ ] **Step 1: Add an exact combined URL RED test**

Append to `internal/mcpserver/combined_parameter_test.go`:

```go
func TestServerBindsPathThenQueryAndPreservesRawServerQuery(t *testing.T) {
    requests := make(chan string, 1)
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
        requests <- request.RequestURI
        w.Header().Set("Content-Type", "application/json")
        _, _ = io.WriteString(w, `{}`)
    }))
    defer upstream.Close()

    operation := combinedOperation(
        upstream.URL + "/api/customers/%7BcustomerId%7D/orders?token=a;b",
    )

    server, err := New(operation, upstream.Client())
    if err != nil {
        t.Fatalf("New() error = %v", err)
    }

    session, cleanup := connectClient(t, server)
    defer cleanup()

    result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
        Name: "getCustomerOrders",
        Arguments: map[string]any{
            "customerId": "a/b",
            "limit":      json.Number("25e0"),
        },
    })
    if err != nil {
        t.Fatalf("CallTool() error = %v", err)
    }
    if result.IsError {
        t.Fatalf("CallTool() IsError = true; content = %#v", result.Content)
    }

    if got := <-requests; got != "/api/customers/a%2Fb/orders?token=a;b&limit=25" {
        t.Fatalf("RequestURI = %q", got)
    }
}
```

Add these imports to the file:

```go
"io"
"net/http/httptest"

"github.com/modelcontextprotocol/go-sdk/mcp"
```

- [ ] **Step 2: Add the combined invalid-input RED matrix**

Append:

```go
func TestServerRejectsInvalidCombinedArgumentsBeforeUpstream(t *testing.T) {
    tests := []struct {
        name      string
        arguments map[string]any
    }{
        {
            name:      "missing path parameter",
            arguments: map[string]any{"limit": 25},
        },
        {
            name:      "missing query parameter",
            arguments: map[string]any{"customerId": "cus_123"},
        },
        {
            name: "null path parameter",
            arguments: map[string]any{
                "customerId": nil,
                "limit":      25,
            },
        },
        {
            name: "null query parameter",
            arguments: map[string]any{
                "customerId": "cus_123",
                "limit":      nil,
            },
        },
        {
            name: "wrong path type",
            arguments: map[string]any{
                "customerId": 123,
                "limit":      25,
            },
        },
        {
            name: "wrong query type",
            arguments: map[string]any{
                "customerId": "cus_123",
                "limit":      "25",
            },
        },
        {
            name: "extra field",
            arguments: map[string]any{
                "customerId": "cus_123",
                "limit":      25,
                "extra":      true,
            },
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            calls := 0
            upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
                calls++
                _, _ = io.WriteString(w, `{}`)
            }))
            defer upstream.Close()

            server, err := New(
                combinedOperation(
                    upstream.URL + "/customers/%7BcustomerId%7D/orders",
                ),
                upstream.Client(),
            )
            if err != nil {
                t.Fatalf("New() error = %v", err)
            }

            session, cleanup := connectClient(t, server)
            defer cleanup()

            result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
                Name:      "getCustomerOrders",
                Arguments: test.arguments,
            })
            if err != nil {
                t.Fatalf("CallTool() protocol error = %v", err)
            }
            if !result.IsError {
                t.Fatalf("CallTool() IsError = false, want true")
            }
            if calls != 0 {
                t.Fatalf("upstream calls = %d, want 0", calls)
            }
        })
    }
}
```

- [ ] **Step 3: Run targeted combined binding tests and verify RED**

Run:

```bash
go test ./internal/mcpserver -run 'TestServer(BindsPathThenQuery|RejectsInvalidCombined)' -count=1
```

Expected: FAIL because `bindOperationArguments` currently chooses the path binder and never binds the query parameter when both pointers are non-nil.

- [ ] **Step 4: Add a bounded combined binder without changing existing single binders**

Change `bindOperationArguments` to:

```go
func bindOperationArguments(
    operation oasopenapi.SelectedOperation,
    input map[string]json.RawMessage,
) (string, error) {
    switch {
    case operation.PathParameter != nil && operation.QueryParameter != nil:
        return bindPathAndQueryParameters(operation, input)
    case operation.PathParameter != nil:
        return bindPathParameter(
            operation.Endpoint,
            operation.PathParameter,
            input,
        )
    default:
        return bindQueryParameter(
            operation.Endpoint,
            operation.QueryParameter,
            input,
        )
    }
}
```

Add:

```go
func bindPathAndQueryParameters(
    operation oasopenapi.SelectedOperation,
    input map[string]json.RawMessage,
) (string, error) {
    pathParameter := operation.PathParameter
    queryParameter := operation.QueryParameter
    if pathParameter == nil || queryParameter == nil {
        return "", fmt.Errorf("combined binding requires one path and one query parameter")
    }
    if pathParameter.Name == queryParameter.Name {
        return "", fmt.Errorf(
            "path and query parameters share MCP input name %q",
            pathParameter.Name,
        )
    }
    if len(input) != 2 {
        return "", fmt.Errorf(
            "tool requires exactly path parameter %q and query parameter %q",
            pathParameter.Name,
            queryParameter.Name,
        )
    }

    pathRaw, ok := input[pathParameter.Name]
    if !ok {
        return "", fmt.Errorf(
            "required path parameter %q is missing",
            pathParameter.Name,
        )
    }
    queryRaw, ok := input[queryParameter.Name]
    if !ok {
        return "", fmt.Errorf(
            "required query parameter %q is missing",
            queryParameter.Name,
        )
    }

    endpoint, err := bindPathParameter(
        operation.Endpoint,
        pathParameter,
        map[string]json.RawMessage{
            pathParameter.Name: pathRaw,
        },
    )
    if err != nil {
        return "", err
    }

    return bindQueryParameter(
        endpoint,
        queryParameter,
        map[string]json.RawMessage{
            queryParameter.Name: queryRaw,
        },
    )
}
```

Do not duplicate primitive parsing, URL query serialization, or path escaping logic.

- [ ] **Step 5: Run the complete MCP package and verify GREEN**

Run:

```bash
gofmt -w internal/mcpserver/server.go internal/mcpserver/combined_parameter_test.go
go test ./internal/mcpserver -count=1
```

Expected: PASS, including all existing parameterless, single-query, single-path, raw-query, explicit-null, integer canonicalization, and path escaping regressions.

- [ ] **Step 6: Commit the binding slice**

```bash
git add internal/mcpserver/server.go internal/mcpserver/combined_parameter_test.go
git commit -m "feat: bind combined path and query arguments"
```

---

### Task 4: Strengthen Docker Acceptance with a Real Combined Call

**Files:**
- Modify: `internal/containertest/docker_stdio_test.go`

**Interfaces:**
- Consumes: existing non-root image assertion, TLS fixture, ephemeral test CA, Bearer environment forwarding, official MCP `CommandTransport`, and secret-leak checks.
- Produces: one container-level proof that the actual image performs combined path/query binding while all existing TLS/Bearer invariants remain enforced.

- [ ] **Step 1: Change the generated Docker acceptance spec to a combined operation**

In `writeSpec`, change the path and operation to:

```yaml
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
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: Customer orders
```

Keep the existing HTTPS `host.docker.internal` server, ephemeral CA mount, and Bearer token setup unchanged.

- [ ] **Step 2: Update the container tool name and arguments**

Change the launched operation ID to:

```text
getCustomerOrders
```

Change the `tools/list` assertion to require exactly that tool.

Call it with:

```go
Arguments: map[string]any{
    "customerId": "cus_123",
    "limit":      25,
},
```

- [ ] **Step 3: Strengthen the observed request assertion to include the query**

Change `observedRequest` from a path-only line to an exact request URI:

```go
type observedRequest struct {
    requestURI    string
    authorization string
}
```

Capture:

```go
requestURI: request.Method + " " + request.URL.RequestURI(),
```

Assert:

```go
if request.requestURI != "GET /api/customers/cus_123/orders?limit=25" {
    t.Fatalf(
        "upstream request = %q, want %q",
        request.requestURI,
        "GET /api/customers/cus_123/orders?limit=25",
    )
}
```

Keep the exact Bearer header assertion and secret non-disclosure assertions unchanged.

- [ ] **Step 4: Run Docker acceptance**

Run:

```bash
./scripts/test-docker.sh
```

Expected: PASS with one stdio MCP tool call reaching the real TLS upstream as:

```text
GET /api/customers/cus_123/orders?limit=25
Authorization: Bearer container-secret
```

The token must remain absent from `tools/list`, MCP tool output, and stderr.

- [ ] **Step 5: Commit the Docker acceptance change**

```bash
git add internal/containertest/docker_stdio_test.go
git commit -m "test: cover combined parameters in docker runtime"
```

---

### Task 5: Document the New Fourth Supported Operation Shape

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: the implemented selector/schema/binding contract.
- Produces: user-facing documentation that states the exact bounded support and does not imply arbitrary multi-parameter support.

- [ ] **Step 1: Update the current-scope summary**

Replace the parameter-support bullet near the top with:

```markdown
- support no parameters, one required primitive operation-level query parameter,
  one required primitive operation-level path parameter, or exactly one of each;
```

- [ ] **Step 2: Update the selected-operation requirements**

Replace:

```markdown
- have either no operation-level parameter or exactly one supported query or path parameter;
```

with:

```markdown
- have no operation-level parameters, one supported query parameter, one supported
  path parameter, or exactly one supported path plus one supported query parameter;
```

- [ ] **Step 3: Add a combined parameter example after the single-path section**

Add:

~~~~markdown
### One required path + one required query parameter

OASRelay also supports exactly one required primitive operation-level path
parameter together with exactly one required primitive operation-level query
parameter.

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
```

The MCP input is a closed object with both properties required:

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

OpenAPI declaration order does not matter. Binding is always path first and
query second. Existing path escaping, raw-query preservation, primitive type
validation, canonical integer serialization, and Bearer security rules remain
unchanged.

This does not enable arbitrary multiple parameters: two query parameters, two
path parameters, more than two operation-level parameters, optional parameters,
headers, cookies, path-item-level inheritance, and custom serialization remain
unsupported.
~~~~

- [ ] **Step 4: Update the implemented/non-goal lists**

In **Implemented**, replace the zero/single bullet with:

```markdown
- zero parameters, one required operation-level primitive query parameter,
  one required operation-level primitive path parameter, or exactly one of each;
```

In **Not implemented**, remove:

```markdown
- query + path parameter combinations;
```

and replace the optional/multiple wording with:

```markdown
- arbitrary multiple operation parameters, including two query parameters or
  two path parameters;
- optional operation parameters;
```

Do not remove the existing header/cookie, inheritance, complex-schema, auth, request-body, multi-tool, transport, or SaaS non-goals.

- [ ] **Step 5: Run documentation-adjacent regression verification**

Run:

```bash
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

Expected: PASS.

- [ ] **Step 6: Commit documentation**

```bash
git add README.md
git commit -m "docs: document combined path and query parameters"
```

---

### Task 6: Final Scope and Verification Gate

**Files:**
- Review only: all files changed by Tasks 1–5.
- Do not add production files during this task.

**Interfaces:**
- Consumes: the complete feature branch.
- Produces: merge-review evidence that Issue #16 is satisfied without widening the project boundary.

- [ ] **Step 1: Run the complete local verification matrix**

```bash
gofmt -w \
  internal/openapi/select.go \
  internal/openapi/combined_parameter_test.go \
  internal/openapi/query_parameter_test.go \
  internal/openapi/path_parameter_test.go \
  internal/mcpserver/server.go \
  internal/mcpserver/combined_parameter_test.go \
  cmd/oasrelay/combined_parameter_test.go \
  internal/containertest/docker_stdio_test.go

go mod tidy
git diff --exit-code -- go.mod go.sum
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
./scripts/test-docker.sh
```

Expected: every command exits 0.

- [ ] **Step 2: Run the explicit scope self-review**

Confirm all of these statements from the final diff:

```text
PASS  GET-only behavior is unchanged.
PASS  Exactly one MCP tool is still exposed.
PASS  SelectedOperation still uses QueryParameter/PathParameter pointers.
PASS  No generic []Parameter or ParameterIR was introduced.
PASS  At most one query and at most one path parameter are accepted.
PASS  Optional/header/cookie/path-item-level parameters remain rejected.
PASS  Complex schemas and custom serialization remain rejected.
PASS  Combined input requires exactly two unique MCP argument names.
PASS  Path binding happens before query binding.
PASS  Existing raw query text is preserved.
PASS  Existing path escaping and integer canonicalization are reused, not duplicated.
PASS  Bearer/TLS/redirect behavior is unchanged.
PASS  Docker acceptance still verifies non-root runtime, TLS, Bearer forwarding,
      secret non-disclosure, and now the combined request URI.
PASS  No security-scheme, multi-tool, write-method, Streamable HTTP,
      configuration, policy, persistence, UI, or SaaS code was added.
```

Any failed statement blocks PR creation until corrected.

- [ ] **Step 3: Review the branch diff**

Run:

```bash
git diff main...HEAD --stat
git diff main...HEAD
```

Expected changed-file set is limited to:

```text
internal/openapi/select.go
internal/openapi/query_parameter_test.go
internal/openapi/path_parameter_test.go
internal/openapi/combined_parameter_test.go
internal/mcpserver/server.go
internal/mcpserver/combined_parameter_test.go
cmd/oasrelay/combined_parameter_test.go
internal/containertest/docker_stdio_test.go
README.md
```

- [ ] **Step 4: Open a draft PR and request code review**

The PR must reference Issue #16, include RED→GREEN evidence for selector/schema/binding, and report the final standard + Docker verification results. Do not merge automatically.

Suggested title:

```text
feat: support one path and one query parameter
```

- [ ] **Step 5: Treat review findings as new gates**

For every substantive review finding:

1. Verify the finding against current code.
2. Add a focused failing regression test when the issue is real.
3. Run the targeted test and capture RED.
4. Implement the smallest fix.
5. Run targeted GREEN.
6. Re-run `go test ./...`, `go vet ./...`, and `./scripts/test-docker.sh`.
7. Resolve the review thread only after the fixing head is green.

## Definition of Done

Issue #16 is done only when the selector accepts the combined shape in either declaration order, rejects every explicitly out-of-scope shape, MCP `tools/list` exposes exactly two required primitive properties, one tool call produces the exact path+query request while preserving existing raw query/path safety, invalid input never reaches upstream, existing single/parameterless/Bearer regressions remain green, Docker proves the combined request over the existing TLS/Bearer runtime, the final diff contains no generalized parameter engine, and the reviewed PR is merged only after final-head CI passes.

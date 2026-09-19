# Optional Single Query Parameter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow the existing single supported operation-level query parameter to be optional while preserving the one-tool GET runtime, current parameter cardinality, existing safety properties, and all required-query behavior.

**Architecture:** Keep the project-owned `SelectedOperation` shape and add only an `Optional bool` flag to `QueryParameter`. The selector maps OpenAPI `required` into that flag, MCP schema generation omits optional query fields from `required`, and existing binding paths are extended so omission preserves the endpoint while presence reuses the current primitive serializer and raw-query append logic. No generic parameter IR, new package, or serialization engine is introduced.

**Tech Stack:** Go 1.25+, kin-openapi, official Model Context Protocol Go SDK, net/http, Docker acceptance testing.

**Spec:** `docs/superpowers/specs/2026-09-19-optional-single-query-parameter-design.md`

## Global Constraints

- GET only.
- Exactly one MCP tool remains exposed.
- Support zero or one operation-level path parameter and zero or one operation-level query parameter.
- Path parameters remain required.
- Query parameters may be required or optional.
- Parameters remain operation-level only; path-item-level parameters remain rejected.
- Supported primitive types remain `string`, `integer`, `number`, and `boolean`.
- Arrays, objects, enum, nullable, unions, defaults, formats, bounds, patterns, and custom serialization remain unsupported.
- MCP input remains a closed object with `additionalProperties: false`.
- Omitted optional query and explicit `null` are different: omission is valid, `null` is invalid.
- Existing raw server query text must remain byte-for-byte preserved.
- Existing canonical integer serialization remains unchanged.
- No upstream HTTP request may occur before argument validation succeeds.
- Existing Bearer/TLS/same-origin redirect behavior remains unchanged.
- Stdio remains the only transport.
- Docker image/runtime invariants remain unchanged.
- Do not introduce a generic parameter collection in this slice.

## File Map

- Modify: `internal/openapi/select.go`
  - Add optionality metadata to the bounded query model and map OpenAPI `required` into it.
- Modify: `internal/openapi/query_parameter_test.go`
  - Prove optional query selection for supported primitive types and omitted/explicit `required: false`.
- Modify: `internal/openapi/combined_parameter_test.go`
  - Prove required path + optional query selection.
- Modify: `cmd/oasrelay/query_parameter_test.go`
  - Prove CLI selection/delegation preserves optional metadata.
- Modify: `internal/mcpserver/server.go`
  - Generate correct MCP `required` lists and bind optional query arguments safely.
- Modify: `internal/mcpserver/query_parameter_test.go`
  - Prove optional-query schema, omission, presence, null/wrong-type/unknown-field failures, and raw-query preservation.
- Modify: `internal/mcpserver/combined_parameter_test.go`
  - Prove required path + optional query schema and binding behavior.
- Modify: `internal/containertest/docker_stdio_test.go`
  - Prove omitted and supplied optional query calls through the real container while retaining TLS/Bearer/non-root/secret assertions.
- Modify: `README.md`
  - Document the new supported shapes and keep non-goals accurate.
- No new production package or production source file should be created.

## Review Focus

1. **Optional query omitted while an existing server raw query contains semicolons or percent escapes** — endpoint must preserve the raw query exactly and add nothing.
2. **Required path present, optional query omitted, unknown extra field supplied** — call must fail before upstream execution rather than silently ignoring the extra field.
3. **Optional query explicitly supplied as `null`** — call must fail as non-null primitive validation, not be treated as omission.
4. **Optional integer supplied as mathematically integral JSON number such as `25e0`** — output must remain canonical `25`.
5. **Existing required-query constructors/tests that rely on Go zero values** — they must remain required; `Optional bool` must default to false.

---

### Task 1: Persist Optionality in the OpenAPI Selection Model

**Files:**
- Modify: `internal/openapi/select.go`
- Modify: `internal/openapi/query_parameter_test.go`
- Modify: `internal/openapi/combined_parameter_test.go`
- Modify: `cmd/oasrelay/query_parameter_test.go`

**Interfaces:**
- Produces:
  ```go
  type QueryParameter struct {
      Name     string
      Type     string
      Optional bool
  }
  ```
- `supportedQueryParameter(operationID string, parameter *openapi3.Parameter) (*QueryParameter, error)` continues to validate all current query restrictions and now records `Optional: !parameter.Required`.
- Existing `SelectedOperation.QueryParameter *QueryParameter` remains unchanged.

- [ ] **Step 1: Add failing selector tests for optional primitive query parameters**

Add a table test in `internal/openapi/query_parameter_test.go` covering `string`, `integer`, `number`, and `boolean` with an operation-level query parameter whose `required` field is omitted.

The core assertion must be:

```go
if got.QueryParameter == nil {
    t.Fatal("QueryParameter = nil")
}
if got.QueryParameter.Name != "filter" ||
    got.QueryParameter.Type != test.schemaType ||
    !got.QueryParameter.Optional {
    t.Fatalf("QueryParameter = %#v", got.QueryParameter)
}
```

Add a second focused case with explicit:

```yaml
required: false
```

and assert the same optional metadata.

Remove the old rejection fixture whose sole expectation is that an optional query parameter "must be required"; all other unsupported-shape fixtures remain.

- [ ] **Step 2: Add failing combined-selector test**

In `internal/openapi/combined_parameter_test.go`, add an operation containing:

```yaml
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
```

Assert:

```go
if got.PathParameter == nil || got.PathParameter.Name != "customerId" {
    t.Fatalf("PathParameter = %#v", got.PathParameter)
}
if got.QueryParameter == nil ||
    got.QueryParameter.Name != "limit" ||
    got.QueryParameter.Type != "integer" ||
    !got.QueryParameter.Optional {
    t.Fatalf("QueryParameter = %#v", got.QueryParameter)
}
```

- [ ] **Step 3: Add failing CLI delegation test**

In `cmd/oasrelay/query_parameter_test.go`, select an optional query operation through `runWithServe` and assert the delegated `SelectedOperation.QueryParameter.Optional` is true.

Do not change CLI syntax.

- [ ] **Step 4: Run targeted tests and prove RED**

Run:

```bash
go test ./internal/openapi ./cmd/oasrelay
```

Expected: FAIL because `QueryParameter.Optional` does not exist and optional query selection is still rejected.

- [ ] **Step 5: Commit the RED evidence**

```bash
git add internal/openapi/query_parameter_test.go internal/openapi/combined_parameter_test.go cmd/oasrelay/query_parameter_test.go
git commit -m "test: specify optional query selection"
```

- [ ] **Step 6: Implement the minimal selector/model change**

Change:

```go
type QueryParameter struct {
    Name string
    Type string
}
```

to:

```go
type QueryParameter struct {
    Name     string
    Type     string
    Optional bool
}
```

Delete only the current rejection:

```go
if !parameter.Required {
    return nil, fmt.Errorf(
        "operationId %q query parameter %q must be required",
        operationID,
        parameter.Name,
    )
}
```

Keep serialization and plain-primitive validation unchanged.

Construct:

```go
return &QueryParameter{
    Name:     parameter.Name,
    Type:     (*parameter.Schema.Value.Type)[0],
    Optional: !parameter.Required,
}, nil
```

- [ ] **Step 7: Run targeted tests and prove GREEN**

Run:

```bash
gofmt -w internal/openapi/select.go internal/openapi/query_parameter_test.go internal/openapi/combined_parameter_test.go cmd/oasrelay/query_parameter_test.go
go test ./internal/openapi ./cmd/oasrelay
```

Expected: PASS.

- [ ] **Step 8: Commit the GREEN implementation**

```bash
git add internal/openapi/select.go internal/openapi/query_parameter_test.go internal/openapi/combined_parameter_test.go cmd/oasrelay/query_parameter_test.go
git commit -m "feat: select optional primitive query parameters"
```

- [ ] **Step 9: Task self-review**

Confirm:

- no parameter collection was introduced;
- required query parameters have `Optional == false`;
- omitted and explicit `required: false` both produce `Optional == true`;
- path parameter behavior is untouched;
- all existing unsupported schema/serialization tests remain present.

Do not proceed if any item fails.

---

### Task 2: Generate Correct MCP Input Schemas

**Files:**
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/query_parameter_test.go`
- Modify: `internal/mcpserver/combined_parameter_test.go`

**Interfaces:**
- Consumes: `QueryParameter.Optional bool` from Task 1.
- Produces: `toolInputSchema(operation SelectedOperation) map[string]any` where optional query properties appear under `properties` but not under `required`.

- [ ] **Step 1: Add failing optional-query schema test**

In `internal/mcpserver/query_parameter_test.go`, construct:

```go
QueryParameter: &oasopenapi.QueryParameter{
    Name:     "limit",
    Type:     "integer",
    Optional: true,
},
```

Call `ListTools`, decode `InputSchema`, and assert:

```go
properties, ok := schema["properties"].(map[string]any)
if !ok {
    t.Fatalf("properties = %#v", schema["properties"])
}
limit, ok := properties["limit"].(map[string]any)
if !ok || limit["type"] != "integer" {
    t.Fatalf("limit schema = %#v", properties["limit"])
}
if _, exists := schema["required"]; exists {
    t.Fatalf("required = %#v, want field omitted", schema["required"])
}
if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
    t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
}
```

- [ ] **Step 2: Add failing path-plus-optional-query schema test**

In `internal/mcpserver/combined_parameter_test.go`, clone the existing combined helper operation and set:

```go
operation.QueryParameter.Optional = true
```

Assert the schema contains both properties but:

```go
required, ok := schema["required"].([]any)
if !ok || len(required) != 1 || required[0] != "customerId" {
    t.Fatalf("required = %#v, want [customerId]", schema["required"])
}
```

- [ ] **Step 3: Run targeted tests and prove RED**

Run:

```bash
go test ./internal/mcpserver -run 'InputSchema|Combined'
```

Expected: FAIL because every present query parameter is currently appended to `required`.

- [ ] **Step 4: Commit the RED evidence**

```bash
git add internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go
git commit -m "test: specify optional query input schemas"
```

- [ ] **Step 5: Implement the minimal schema change**

In `toolInputSchema`, keep the property creation unchanged and change only required-list behavior:

```go
if parameter := operation.QueryParameter; parameter != nil {
    properties[parameter.Name] = map[string]any{
        "type": parameter.Type,
    }
    if !parameter.Optional {
        required = append(required, parameter.Name)
    }
}
```

Do not add nullable/default metadata.

- [ ] **Step 6: Run targeted tests and prove GREEN**

Run:

```bash
gofmt -w internal/mcpserver/server.go internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go
go test ./internal/mcpserver -run 'InputSchema|Combined'
```

Expected: PASS.

Then run the complete package:

```bash
go test ./internal/mcpserver
```

Expected: PASS, including existing required-query schema tests.

- [ ] **Step 7: Commit the GREEN implementation**

```bash
git add internal/mcpserver/server.go internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go
git commit -m "feat: expose optional query fields in MCP schema"
```

- [ ] **Step 8: Task self-review**

Confirm:

- optional query property is still visible to MCP clients;
- optional query is absent only from `required`;
- required query remains in `required`;
- required path remains in `required`;
- `additionalProperties: false` is unchanged.

Do not proceed if any item fails.

---

### Task 3: Bind Omitted and Supplied Optional Query Arguments Safely

**Files:**
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/query_parameter_test.go`
- Modify: `internal/mcpserver/combined_parameter_test.go`
- Existing regression coverage: `internal/mcpserver/base_query_test.go`

**Interfaces:**
- Consumes: `QueryParameter.Optional`.
- Preserves: `primitiveQueryValue`, `bindPathParameter`, URL query escaping, raw-query append logic.
- Produces:
  - optional query omitted => unchanged endpoint;
  - optional query supplied => validated and appended;
  - required path + optional query omitted => path-bound endpoint only.

- [ ] **Step 1: Add failing optional-query execution tests**

In `internal/mcpserver/query_parameter_test.go`, add an optional query operation and prove both calls:

Omitted:

```go
result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
    Name:      "listCustomers",
    Arguments: map[string]any{},
})
```

Expected upstream request URI contains no `limit` pair.

Supplied:

```go
result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
    Name: "listCustomers",
    Arguments: map[string]any{
        "limit": json.Number("25e0"),
    },
})
```

Expected `limit=25`.

Use a raw-query fixture such as:

```text
/customers?token=a;b&mode=raw%2Fvalue
```

and assert exact request URIs:

```text
/customers?token=a;b&mode=raw%2Fvalue
/customers?token=a;b&mode=raw%2Fvalue&limit=25
```

- [ ] **Step 2: Add failing invalid-input tests for optional query**

Add cases:

```go
{name: "explicit null", arguments: map[string]any{"limit": nil}},
{name: "wrong primitive type", arguments: map[string]any{"limit": "25"}},
{name: "unknown field", arguments: map[string]any{"cursor": "next"}},
{name: "optional plus unknown field", arguments: map[string]any{"limit": 25, "cursor": "next"}},
```

Each case must assert:

```go
if !result.IsError {
    t.Fatalf("CallTool() IsError = false, want true")
}
if calls.Load() != 0 {
    t.Fatalf("upstream calls = %d, want 0", calls.Load())
}
```

- [ ] **Step 3: Add failing required-path-plus-optional-query execution tests**

In `internal/mcpserver/combined_parameter_test.go`, set:

```go
operation.QueryParameter.Optional = true
```

Prove:

1. `{"customerId":"cus_123"}` => `/customers/cus_123/orders`;
2. `{"customerId":"cus_123","limit":25}` => `/customers/cus_123/orders?limit=25`;
3. `{"limit":25}` => error, zero upstream calls;
4. `{"customerId":"cus_123","extra":true}` => error, zero upstream calls;
5. `{"customerId":"cus_123","limit":null}` => error, zero upstream calls.

- [ ] **Step 4: Run targeted tests and prove RED**

Run:

```bash
go test ./internal/mcpserver
```

Expected: FAIL because `bindQueryParameter` currently requires exactly one query argument and `bindPathAndQueryParameters` currently requires exactly two arguments.

- [ ] **Step 5: Commit the RED evidence**

```bash
git add internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go
git commit -m "test: specify optional query binding"
```

- [ ] **Step 6: Implement optional query-only binding**

Keep the `parameter == nil` parameterless branch unchanged.

Before decoding the query value, implement:

```go
if parameter.Optional && len(input) == 0 {
    return endpoint, nil
}

if len(input) != 1 {
    if parameter.Optional {
        return "", fmt.Errorf(
            "tool accepts at most optional query parameter %q",
            parameter.Name,
        )
    }
    return "", fmt.Errorf(
        "tool requires exactly query parameter %q",
        parameter.Name,
    )
}

raw, ok := input[parameter.Name]
if !ok {
    if parameter.Optional {
        return "", fmt.Errorf(
            "tool accepts only optional query parameter %q",
            parameter.Name,
        )
    }
    return "", fmt.Errorf(
        "required query parameter %q is missing",
        parameter.Name,
    )
}
```

Then leave `primitiveQueryValue`, URL parsing, query encoding, raw-query append, and integer canonicalization unchanged.

- [ ] **Step 7: Implement required-path-plus-optional-query binding**

Preserve duplicate-name validation.

Require the path argument first:

```go
pathRaw, ok := input[pathParameter.Name]
if !ok {
    return "", fmt.Errorf(
        "required path parameter %q is missing",
        pathParameter.Name,
    )
}
```

Then distinguish the optional query shape without permitting unknown fields:

```go
queryRaw, queryPresent := input[queryParameter.Name]

if queryParameter.Optional {
    switch {
    case len(input) == 1 && !queryPresent:
        return bindPathParameter(
            operation.Endpoint,
            pathParameter,
            map[string]json.RawMessage{
                pathParameter.Name: pathRaw,
            },
        )
    case len(input) != 2 || !queryPresent:
        return "", fmt.Errorf(
            "tool requires path parameter %q and accepts only optional query parameter %q",
            pathParameter.Name,
            queryParameter.Name,
        )
    }
} else {
    if len(input) != 2 {
        return "", fmt.Errorf(
            "tool requires exactly path parameter %q and query parameter %q",
            pathParameter.Name,
            queryParameter.Name,
        )
    }
    if !queryPresent {
        return "", fmt.Errorf(
            "required query parameter %q is missing",
            queryParameter.Name,
        )
    }
}
```

After validation, keep binding order path first, then query.

The implementation may factor repeated map construction only if the resulting helper remains local and narrower than the existing functions; do not introduce a generic parameter-validation subsystem.

- [ ] **Step 8: Run targeted tests and prove GREEN**

Run:

```bash
gofmt -w internal/mcpserver/server.go internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go
go test ./internal/mcpserver
```

Expected: PASS.

Also run the raw-query regression directly:

```bash
go test ./internal/mcpserver -run 'Raw|BaseQuery|Optional'
```

Expected: PASS.

- [ ] **Step 9: Commit the GREEN implementation**

```bash
git add internal/mcpserver/server.go internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go
git commit -m "feat: bind optional query arguments"
```

- [ ] **Step 10: Task self-review**

Confirm:

- omitted optional query does not parse/re-encode a query-only endpoint;
- path + omitted optional query binds only the path;
- explicit null is still routed into `primitiveQueryValue` and rejected;
- unknown fields cannot be silently ignored;
- required-query behavior has not been relaxed;
- all validation failures happen before `upstream.Get`.

Do not proceed if any item fails.

---

### Task 4: Strengthen Docker End-to-End Acceptance

**Files:**
- Modify: `internal/containertest/docker_stdio_test.go`

**Interfaces:**
- Consumes the real built image, stdio MCP transport, generated OpenAPI spec, TLS fixture, Bearer environment variable.
- Proves the same optional query operation works both omitted and supplied through the container.

- [ ] **Step 1: Make the generated query parameter optional in the acceptance fixture**

Change only the query parameter declaration:

```yaml
- name: limit
  in: query
  required: false
  schema:
    type: integer
```

Keep the required path parameter unchanged.

- [ ] **Step 2: Extend the request observation capacity**

Change:

```go
requests := make(chan observedRequest, 1)
```

to:

```go
requests := make(chan observedRequest, 2)
```

The expected upstream call count becomes two.

- [ ] **Step 3: Add the omitted optional-query MCP call**

Call:

```go
result, err := session.CallTool(ctx, &mcp.CallToolParams{
    Name: "getCustomerOrders",
    Arguments: map[string]any{
        "customerId": "cus_123",
    },
})
```

Assert success and the existing structured response.

Read the first observed request and require:

```text
GET /api/customers/cus_123/orders
```

Also require the existing Bearer header.

- [ ] **Step 4: Keep and adapt the supplied optional-query MCP call**

Call:

```go
result, err = session.CallTool(ctx, &mcp.CallToolParams{
    Name: "getCustomerOrders",
    Arguments: map[string]any{
        "customerId": "cus_123",
        "limit":      25,
    },
})
```

Require:

```text
GET /api/customers/cus_123/orders?limit=25
```

and the same Bearer header.

Keep secret non-disclosure checks against `tools/list`, tool results, and stderr.

Require:

```go
if calls.Load() != 2 {
    t.Fatalf("upstream calls = %d, want 2", calls.Load())
}
```

- [ ] **Step 5: Run the container acceptance and prove GREEN**

Build and run using the repository's existing contract:

```bash
./scripts/test-docker.sh
```

Expected: PASS, including non-root user, TLS trust, Bearer forwarding, optional omission, optional presence, secret non-disclosure, and exactly two upstream calls.

If the repository's script is not executable on the current platform, run the exact equivalent command documented by the repository without weakening assertions.

- [ ] **Step 6: Commit the acceptance strengthening**

```bash
git add internal/containertest/docker_stdio_test.go
git commit -m "test: cover optional query in Docker acceptance"
```

- [ ] **Step 7: Task self-review**

Confirm the Docker test still proves:

- user `65532:65532`;
- stdio MCP transport;
- generated local spec mounted read-only;
- HTTPS upstream trust;
- Bearer forwarding;
- no Bearer token disclosure;
- omitted and supplied optional-query calls;
- exact upstream URLs;
- exactly two upstream requests.

Do not proceed if any item fails.

---

### Task 5: Align Documentation and Run Full Verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- Documents the shipped contract only; do not advertise multiple queries, generic optional parameters, defaults, nullable, or security-scheme processing.

- [ ] **Step 1: Update the current-scope summary**

Replace language that says the query parameter must always be required with wording equivalent to:

```text
support no parameters, one primitive operation-level query parameter
(required or optional), one required primitive operation-level path
parameter, or exactly one path plus one query parameter
```

- [ ] **Step 2: Add an optional-query example**

Document an operation such as:

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
```

Document both calls:

```json
{}
```

=> no `limit` query pair.

```json
{"limit": 25}
```

=> `?limit=25`.

Explicitly state that `null` is invalid and schema defaults remain unsupported.

- [ ] **Step 3: Update path-plus-query documentation**

State that the single query parameter paired with the required path may be required or optional, while the path remains required.

- [ ] **Step 4: Correct the non-goals list**

Remove blanket wording that all optional operation parameters are unsupported.

Retain explicit non-goals:

- optional path parameters;
- multiple query/path parameters;
- headers/cookies;
- defaults;
- nullable/complex schemas;
- OpenAPI security schemes;
- multiple MCP tools.

- [ ] **Step 5: Run formatting and module hygiene**

Run:

```bash
gofmt -w internal/openapi/select.go internal/openapi/query_parameter_test.go internal/openapi/combined_parameter_test.go cmd/oasrelay/query_parameter_test.go internal/mcpserver/server.go internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go internal/containertest/docker_stdio_test.go
go mod tidy
git diff --exit-code -- go.mod go.sum
```

Expected: formatting succeeds; `go mod tidy` produces no dependency drift.

- [ ] **Step 6: Run the complete Go verification matrix**

Run:

```bash
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

Expected:

- all tests PASS;
- vet PASS;
- inspect smoke succeeds with the existing deterministic output contract.

- [ ] **Step 7: Run Docker acceptance again from the final tree**

Run:

```bash
./scripts/test-docker.sh
```

Expected: PASS.

- [ ] **Step 8: Commit documentation**

```bash
git add README.md
git commit -m "docs: document optional query parameters"
```

- [ ] **Step 9: Perform full scope self-review**

Check the final diff against the spec:

- no second query parameter support;
- no second path parameter support;
- no optional path support;
- no new schema features;
- no default injection;
- no generic parameter IR;
- no auth/security-scheme changes;
- no multiple-tool changes;
- required-query regressions remain green;
- all validation failures remain pre-network;
- raw-query preservation remains exact.

If any item fails, fix it before external review.

- [ ] **Step 10: Produce final verification evidence**

Record:

- final branch name;
- final commit SHA;
- targeted RED/GREEN commit pairs for Tasks 1–3;
- `go test ./...` result;
- `go vet ./...` result;
- inspect smoke result;
- Docker acceptance result;
- scope self-review result.

Do not claim the implementation complete without fresh final-tree evidence.

---

## Plan Self-Review

### 1. Spec Coverage

Covered:

- optional query selection and project-owned metadata — Task 1;
- zero-value backward safety through `Optional bool` — Task 1;
- MCP required-list semantics — Task 2;
- omitted versus null — Task 3;
- optional-only and path+optional binding — Task 3;
- raw query preservation — Task 3;
- canonical integer behavior — Task 3;
- validation before network — Task 3;
- Docker/TLS/Bearer/non-root acceptance — Task 4;
- documentation and full verification — Task 5;
- non-goals and scope boundary — Tasks 1–5 self-review gates.

No spec requirement is uncovered.

### 2. Placeholder Scan

No TBD/TODO/implement-later placeholders are present. Every production behavior step names the exact existing function or field to modify and provides the intended logic.

### 3. Type Consistency

The plan consistently uses:

```go
type QueryParameter struct {
    Name     string
    Type     string
    Optional bool
}
```

No task uses the rejected `Required bool` design.

### 4. Review Focus Coverage

- raw-query omission/preservation — Task 3;
- path + optional + unknown extra field — Task 3;
- explicit null — Task 3;
- canonical `25e0 -> 25` — Task 3;
- zero-value required compatibility — Tasks 1 and 2 regression runs.

All five high-risk inputs have owning tests.

## Execution Gate

Implementation must not start until this plan is reviewed.

Recommended execution mode: **subagent-driven**, because Tasks 1–4 each change a distinct contract boundary (selection, MCP schema, binding, container E2E) and benefit from an independent review/self-review before the next boundary is opened. If execution remains native instead, keep the same RED → GREEN → self-review gates and do not batch multiple tasks into one implementation step.

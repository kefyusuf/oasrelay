# Second Primitive Query Parameter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support exactly two primitive operation-level query parameters for query-only GET operations while keeping the total operation-level parameter cap at two and preserving the one-tool least-authority runtime.

**Architecture:** Migrate the project-owned query representation from one pointer to a bounded `[]QueryParameter` with semantic cardinality `0..2`. First migrate all existing one-query consumers without changing behavior, then open two-query selection, then atomically open MCP schema plus runtime binding so no intermediate task exposes an executable two-query tool with incomplete binding. Docker and README work follow only after in-process behavior is green.

**Tech Stack:** Go 1.25, kin-openapi, official MCP Go SDK, net/http/net/url, Docker, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-30-second-primitive-query-parameter-design.md`

## Execution Checkpoint — 2026-10-02

Implementation is complete on `feat/second-primitive-query-parameter`. [PR #19](https://github.com/kefyusuf/oasrelay/pull/19) is open and ready for review, not merged. The detailed steps below retain the original approved plan; use this checkpoint to determine remaining work.

- [x] Task 1: bounded query model migration.
- [x] Task 2: selection of two primitive query parameters.
- [x] Task 3: atomic MCP schema and query binding support.
- [x] Task 4: dedicated Docker acceptance, retaining the existing path-plus-query test.
- [x] Task 5: README and final Go/Docker verification.

Verified implementation checkpoint: `1d1d277cc506710a1ca0f4c9f3b20dd0d7be817f`. [CI run 36975916031](https://github.com/kefyusuf/oasrelay/actions/runs/36975916031) passed both `verify` and `docker-acceptance` on this exact commit. Local `go test ./...`, `go vet ./...`, inspect smoke, container-tagged compilation/vet, and module consistency also passed. Docker is unavailable on the current Windows host; real container execution was verified in CI.

Implementation commits:

- Task 1 final model migration: `910c6d1`.
- Task 2 selector: `908b31f`.
- Task 3 MCP schema/binding: `e2e310d`.
- Task 4 Docker acceptance: `21e2f5e`.
- Task 5 README: `1d1d277`.

The PR records earlier TDD RED/GREEN evidence. An independent source review of `1d1d277` found no actionable correctness or regression issues in selection, schema, binding, validation, or the preserved path/query/Bearer boundaries; that review did not rerun tests. Automatic review services did not provide a substantive code review: CodeRabbit skipped review and Qodo reported reviews paused. The remaining boundary is maintainer review and merge. No new feature scope or merge is authorized by this checkpoint.

Before resuming, verify the checkout branch, HEAD, working-tree changes, and PR state. Do not repeat Tasks 1–5 simply because the original step checkboxes remain unchecked.

## Global Constraints

- GET only.
- Exactly one selected `operationId` and exactly one MCP tool.
- Total operation-level parameter cap remains exactly two.
- Query parameter cardinality is `0..2`.
- Path parameter cardinality remains `0..1`.
- A path parameter remains required.
- Path + two queries is unsupported.
- Three or more queries are unsupported.
- Operation-level parameters only; path-item parameter inheritance remains unsupported.
- Query schemas remain plain primitive `string`, `integer`, `number`, or `boolean`.
- Arrays, objects, enum, nullable, unions, schema constraints, defaults, and custom serialization remain unsupported.
- Query binding order is OpenAPI declaration order.
- Existing raw server query text must remain byte-for-byte preserved.
- No generic `Parameter` IR or generic parameter subsystem.
- No OpenAPI `securitySchemes`, OAuth/OIDC, API-key, Basic-auth, or secret-resolution work.
- No multiple MCP tools, Streamable HTTP, request bodies, or non-GET execution.
- Existing Bearer HTTPS, same-origin redirect, timeout, response-size, non-root Docker, and secret non-disclosure invariants remain unchanged.

## Review Focus

These are the highest-risk edge conditions implied by the spec. Each is pinned to a task below.

1. **Requiredness must not become positional:** optional-first + required-second with the first omitted must succeed and bind only the second query. Task 2 proves selection order/metadata; Task 3 proves schema and runtime.
2. **Explicit empty string is supplied data, not omission:** an optional string query with `""` must serialize as `name=`. Task 3 owns this test.
3. **Duplicate query names must not collapse in maps:** two selected query contracts with the same name must be rejected before tool registration/network. Task 2 tests OpenAPI selection; Task 3 tests defensive runtime model validation.
4. **Malformed internal model path + two queries must not accidentally execute:** `mcpserver.New` must reject it even though the OpenAPI selector cannot produce it. Task 3 owns this test.
5. **Both optional queries omitted with an existing raw server query must return the endpoint unchanged:** no parse/re-encode round trip. Task 3 owns the exact raw-query regression.

---

### Task 1: Migrate the Query Model to a Bounded Slice Without Adding New Behavior

**Files:**
- Modify: `internal/openapi/select.go`
- Modify: `internal/openapi/query_parameter_test.go`
- Modify: `internal/openapi/path_parameter_test.go`
- Modify: `internal/openapi/combined_parameter_test.go`
- Modify: `cmd/oasrelay/query_parameter_test.go`
- Modify: `cmd/oasrelay/path_parameter_test.go`
- Modify: `cmd/oasrelay/combined_parameter_test.go`
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/query_parameter_test.go`
- Modify: `internal/mcpserver/combined_parameter_test.go`
- Modify: `internal/mcpserver/base_query_test.go`

**Interfaces:**
- Consumes existing:
  - `type QueryParameter struct { Name string; Type string; Optional bool }`
  - `type PathParameter struct { Name string; Type string }`
  - `SelectGET(path, operationID string) (SelectedOperation, error)`
- Produces:
  - `SelectedOperation.QueryParameters []QueryParameter`
  - semantic cardinality `0..2`, but this task preserves executable behavior at `0..1`;
  - existing one-query, path-only, and path+one-query behavior remains unchanged.
- Temporary execution guard for this task:
  - `mcpserver.New` rejects `len(operation.QueryParameters) > 1`.
  - Task 3 removes this guard only when schema and binding support are added atomically.

- [ ] **Step 1: Migrate tests to the new field and keep every existing semantic assertion**

Update direct references from:

```go
operation.QueryParameter
```

to bounded slice assertions such as:

```go
if len(got.QueryParameters) != 1 {
    t.Fatalf("len(QueryParameters) = %d, want 1", len(got.QueryParameters))
}
parameter := got.QueryParameters[0]
if parameter.Name != "limit" || parameter.Type != "integer" || parameter.Optional {
    t.Fatalf("QueryParameters = %#v", got.QueryParameters)
}
```

Path-only tests must assert:

```go
if len(got.QueryParameters) != 0 {
    t.Fatalf("QueryParameters = %#v, want none", got.QueryParameters)
}
```

Manual MCP operations become:

```go
QueryParameters: []oasopenapi.QueryParameter{
    {Name: "limit", Type: "integer"},
},
```

Do not add two-query success tests yet.

- [ ] **Step 2: Run the migration tests and prove RED**

Run:

```bash
go test ./internal/openapi ./internal/mcpserver ./cmd/oasrelay
```

Expected: compile failure because `SelectedOperation.QueryParameters` does not exist and the old `QueryParameter` field is still present.

Formatting errors do not count as valid RED.

- [ ] **Step 3: Commit RED migration evidence**

```bash
git add internal/openapi/*parameter_test.go cmd/oasrelay/*parameter_test.go internal/mcpserver/*parameter_test.go internal/mcpserver/base_query_test.go
git commit -m "test: specify bounded query parameter model"
```

- [ ] **Step 4: Replace the project-owned field in `internal/openapi/select.go`**

Use:

```go
type SelectedOperation struct {
    OperationID     string
    Method          string
    Path            string
    Summary         string
    Description     string
    Endpoint        string
    QueryParameters []QueryParameter
    PathParameter   *PathParameter
}
```

Preserve `QueryParameter` and `PathParameter` types unchanged.

Change internal selection/build signatures from one query pointer to the bounded slice, but continue rejecting a second query in this task.

The task's selector behavior remains exactly the pre-task behavior: zero or one query only.

- [ ] **Step 5: Migrate MCP server consumers without enabling two-query execution**

In `internal/mcpserver/server.go`:

- read zero/one query from `operation.QueryParameters`;
- preserve existing one-query schema behavior;
- preserve existing query-only and path+query binding behavior;
- reject `len(operation.QueryParameters) > 1` from `New` with a defensive model-cardinality error;
- preserve duplicate path/query name validation for the one-query combined shape.

Do not loop over two query parameters in schema or binding yet.

Keep the existing single-query primitive binder behavior intact. Renaming `bindQueryParameter` is not part of this task.

- [ ] **Step 6: Run targeted migration verification**

Run:

```bash
gofmt -w internal/openapi/select.go internal/openapi/*parameter_test.go cmd/oasrelay/*parameter_test.go internal/mcpserver/server.go internal/mcpserver/*parameter_test.go internal/mcpserver/base_query_test.go
go test ./internal/openapi ./internal/mcpserver ./cmd/oasrelay
```

Expected: PASS.

- [ ] **Step 7: Run the full suite**

Run:

```bash
go test ./...
go vet ./...
```

Expected: PASS.

- [ ] **Step 8: Commit the model migration**

```bash
git add internal/openapi/select.go internal/openapi/*parameter_test.go cmd/oasrelay/*parameter_test.go internal/mcpserver/server.go internal/mcpserver/*parameter_test.go internal/mcpserver/base_query_test.go
git commit -m "refactor: migrate query parameters to bounded slice"
```

- [ ] **Step 9: Task self-review**

Confirm:

- old `SelectedOperation.QueryParameter` no longer exists;
- all existing one-query behavior remains green;
- path-only operations carry an empty query slice;
- path+one-query still works;
- `mcpserver.New` explicitly blocks two-query execution for now;
- no generic parameter type or new capability was introduced.

---

### Task 2: Select Exactly Two Primitive Query Parameters in Declaration Order

**Files:**
- Modify: `internal/openapi/select.go`
- Modify: `internal/openapi/query_parameter_test.go`
- Modify: `internal/openapi/combined_parameter_test.go`
- Modify: `cmd/oasrelay/query_parameter_test.go`

**Interfaces:**
- Consumes `SelectedOperation.QueryParameters []QueryParameter` from Task 1.
- Produces:
  - selector supports zero, one, or two query parameters;
  - query order equals OpenAPI operation declaration order;
  - total operation parameter cap remains two;
  - path + one query remains supported;
  - path + two queries remains impossible under the unchanged total cap.

- [ ] **Step 1: Add RED tests for two-query selection and order**

Add a table-driven selector test covering all requiredness combinations:

```text
required + required
required + optional
optional + required
optional + optional
```

Use declaration order `limit` then `cursor`.

Assert:

```go
if len(got.QueryParameters) != 2 {
    t.Fatalf("len(QueryParameters) = %d, want 2", len(got.QueryParameters))
}
if got.QueryParameters[0].Name != "limit" ||
    got.QueryParameters[1].Name != "cursor" {
    t.Fatalf("QueryParameters = %#v", got.QueryParameters)
}
```

Assert each element's `Optional` bit independently.

The `optional + required` case is Review Focus #1 and must remain in declaration order even though only the second field is required.

- [ ] **Step 2: Add RED negative-shape tests**

Prove:

- three query parameters => existing total-cap rejection;
- one path + two query parameters => existing total-cap rejection;
- two path parameters => existing path-cardinality rejection;
- duplicate query names => explicit duplicate-name rejection;
- header/cookie remain rejected;
- unsupported schema/default/custom serialization remain rejected by existing policies.

For duplicate query names, use two operation-level query parameters named `filter` with otherwise valid primitive schemas.

- [ ] **Step 3: Add CLI selection coverage**

Add:

```go
func TestRunServeSelectsTwoPrimitiveQueryParameters(t *testing.T)
```

The callback passed to `runWithServe` must receive two query contracts in declaration order.

Do not start MCP runtime execution in this test; Task 3 owns executable two-query behavior.

- [ ] **Step 4: Run targeted tests and prove RED**

Run:

```bash
go test ./internal/openapi ./cmd/oasrelay
```

Expected: FAIL because the selector still rejects the second query parameter.

- [ ] **Step 5: Commit RED selector evidence**

```bash
git add internal/openapi/query_parameter_test.go internal/openapi/combined_parameter_test.go cmd/oasrelay/query_parameter_test.go
git commit -m "test: specify two query parameter selection"
```

- [ ] **Step 6: Implement bounded two-query collection**

Change the internal selector interface to:

```go
func supportedOperationParameters(
    operationID, route string,
    parameters openapi3.Parameters,
) ([]QueryParameter, *PathParameter, error)
```

Collect query parameters by appending in the original `parameters` iteration order.

Rules:

- reject more than two query parameters;
- reject more than one path parameter;
- reject duplicate names among query parameters;
- reject a query name equal to the selected path parameter name;
- call existing `supportedQueryParameter` for each query so primitive/default/serialization policy remains centralized;
- do not increase `len(operation.Parameters) > 2`.

Update `buildSelectedOperation` to accept `[]QueryParameter`.

- [ ] **Step 7: Run selector verification**

Run:

```bash
gofmt -w internal/openapi/select.go internal/openapi/query_parameter_test.go internal/openapi/combined_parameter_test.go cmd/oasrelay/query_parameter_test.go
go test ./internal/openapi ./cmd/oasrelay
```

Expected: PASS.

- [ ] **Step 8: Run full regression suite**

Run:

```bash
go test ./...
go vet ./...
```

Expected: PASS. The MCP runtime may still reject a selected two-query operation when actually served; Task 1's explicit execution guard remains until Task 3.

- [ ] **Step 9: Commit selector GREEN**

```bash
git add internal/openapi/select.go internal/openapi/query_parameter_test.go internal/openapi/combined_parameter_test.go cmd/oasrelay/query_parameter_test.go
git commit -m "feat: select two primitive query parameters"
```

- [ ] **Step 10: Task self-review**

Confirm:

- declaration order is preserved;
- all four requiredness combinations are represented correctly;
- the total parameter cap is still two;
- path + two queries is not enabled;
- no MCP runtime behavior was opened in this task.

---

### Task 3: Expose and Bind Two Query Parameters Atomically

**Files:**
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/query_parameter_test.go`
- Modify: `internal/mcpserver/combined_parameter_test.go`
- Modify: `internal/mcpserver/base_query_test.go`

**Interfaces:**
- Consumes `SelectedOperation.QueryParameters []QueryParameter` with semantic cardinality `0..2`.
- Produces:
  - `toolInputSchema` exposes both query properties and correct requiredness;
  - `bindQueryParameters(endpoint string, parameters []oasopenapi.QueryParameter, input map[string]json.RawMessage) (string, error)`;
  - query binding order equals slice/declaration order;
  - existing path + exactly one query uses the same bounded query binder;
  - malformed model states are rejected by `New`.

- [ ] **Step 1: Add RED schema tests for all requiredness combinations**

Add a two-query operation helper using `limit` then `cursor`.

For each combination assert:

- both properties exist;
- property types match;
- `additionalProperties == false`;
- required + required => `required == ["limit", "cursor"]`;
- required + optional => `required == ["limit"]`;
- optional + required => `required == ["cursor"]`;
- optional + optional => the `required` field is absent.

The optional-first + required-second case is Review Focus #1.

- [ ] **Step 2: Add RED runtime tests for exact declaration-order binding**

Use an upstream endpoint containing:

```text
/customers?token=a;b&mode=raw%2Fvalue
```

For required `limit` then optional `cursor`:

```json
{"limit": 25}
```

must produce:

```text
/customers?token=a;b&mode=raw%2Fvalue&limit=25
```

and:

```json
{"limit": 25, "cursor": "next page"}
```

must produce:

```text
/customers?token=a;b&mode=raw%2Fvalue&limit=25&cursor=next+page
```

Use a second numeric query in at least one case with `json.Number("25e0")` to prove canonical integer serialization applies independently.

- [ ] **Step 3: Add RED omission and empty-string tests**

For two optional queries `limit` and `filter`:

- `{}` => endpoint/raw query unchanged exactly;
- `{"limit": 25}` => append only `limit=25`;
- `{"filter": ""}` => append explicit `filter=`;
- both supplied => append in declaration order.

The empty-string case is Review Focus #2. Both-omitted raw-query preservation is Review Focus #5.

- [ ] **Step 4: Add RED invalid-input tests with zero upstream calls**

Cover:

- missing first required query;
- missing second required query;
- explicit `null` in either query;
- wrong primitive type in either query;
- unknown field;
- valid selected queries plus an extra field.

Each case must assert `result.IsError == true` and upstream calls remain zero.

- [ ] **Step 5: Add RED defensive model-state tests**

Prove `New` rejects:

1. three query contracts;
2. path + two query contracts;
3. duplicate query contract names;
4. a path name equal to the one selected query name.

Cases 2 and 3 are Review Focus #3 and #4.

- [ ] **Step 6: Run targeted tests and prove RED**

Run:

```bash
go test ./internal/mcpserver
```

Expected: FAIL because Task 1's guard rejects two queries and schema/binding only understand one query.

- [ ] **Step 7: Commit RED runtime evidence**

```bash
git add internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go internal/mcpserver/base_query_test.go
git commit -m "test: specify two query parameter runtime"
```

- [ ] **Step 8: Implement defensive selected-operation validation in `New`**

Validate before tool registration:

- `len(QueryParameters) <= 2`;
- if `PathParameter != nil`, then `len(QueryParameters) <= 1`;
- query names are unique;
- query/path input names are unique.

Remove Task 1's temporary `>1` execution guard only after these final constraints are in place.

Do not add a generic validation subsystem.

- [ ] **Step 9: Update `toolInputSchema` to iterate the bounded query slice**

Keep path first.

Then iterate query contracts in slice order:

```go
for _, parameter := range operation.QueryParameters {
    properties[parameter.Name] = map[string]any{"type": parameter.Type}
    if !parameter.Optional {
        required = append(required, parameter.Name)
    }
}
```

Preserve omission of the `required` field when the final required list is empty.

- [ ] **Step 10: Introduce the bounded query binder**

Use the exact internal interface:

```go
func bindQueryParameters(
    endpoint string,
    parameters []oasopenapi.QueryParameter,
    input map[string]json.RawMessage,
) (string, error)
```

Behavior:

1. reject `len(parameters) > 2`;
2. validate input names against selected query names;
3. require every non-optional query;
4. validate each supplied value through existing `primitiveQueryValue`;
5. build encoded operation pairs in parameter slice order;
6. if zero pairs are supplied, return `endpoint` directly;
7. otherwise parse the endpoint once and append each encoded pair to `RawQuery` in slice order.

For each pair, using:

```go
url.Values{parameter.Name: []string{value}}.Encode()
```

is allowed because it encodes only one key at a time.

Do not put both selected query keys into one `url.Values`.

- [ ] **Step 11: Route existing operation shapes through the bounded binder**

`bindOperationArguments`:

- parameterless/query-only => `bindQueryParameters(operation.Endpoint, operation.QueryParameters, input)`;
- path-only => existing `bindPathParameter`;
- path + one query => existing path binding first, then `bindQueryParameters` with the single query contract.

Simplify `bindPathAndQueryParameters` only as needed to consume the slice; do not add path + two-query behavior.

Remove or retain `bindQueryParameter` only if the final code has one clear query-binding implementation. If retained, it must be a narrow wrapper around `bindQueryParameters`, not a second behavioral path.

- [ ] **Step 12: Run targeted GREEN verification**

Run:

```bash
gofmt -w internal/mcpserver/server.go internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go internal/mcpserver/base_query_test.go
go test ./internal/mcpserver
```

Expected: PASS.

Also run:

```bash
go test ./internal/mcpserver -run 'Query|Raw|Optional|Combined'
```

Expected: PASS.

- [ ] **Step 13: Run full suite and vet**

Run:

```bash
go test ./...
go vet ./...
```

Expected: PASS.

- [ ] **Step 14: Commit runtime GREEN**

```bash
git add internal/mcpserver/server.go internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go internal/mcpserver/base_query_test.go
git commit -m "feat: bind two primitive query parameters"
```

- [ ] **Step 15: Task self-review**

Confirm:

- two query fields are exposed only for query-only operations;
- requiredness is derived per element, not by position;
- `optional + required` works with the first omitted;
- empty string remains explicit data;
- both optional omitted returns the exact endpoint string;
- declaration order, not map/alphabetic order, controls output;
- invalid input is rejected before `upstream.Get`;
- malformed internal path+two-query state is rejected before registration;
- path+one-query regressions remain green.

---

### Task 4: Add Real Docker Acceptance for the Two-Query Shape

**Files:**
- Modify: `internal/containertest/docker_stdio_test.go`

**Interfaces:**
- Consumes the real built image, stdio MCP transport, generated OpenAPI fixtures, HTTPS test CA, and process-level Bearer env configuration.
- Preserves the existing required-path + optional-query acceptance unchanged.
- Produces container evidence for required `limit` + optional `cursor`.

- [ ] **Step 1: Preserve the existing Docker acceptance test**

Do not remove or weaken the existing path + optional-query assertions:

- user `65532:65532`;
- read-only spec/CA mounts;
- stdio MCP;
- TLS trust;
- Bearer forwarding;
- token non-disclosure;
- omitted/supplied optional query;
- exact upstream request URI.

- [ ] **Step 2: Add a dedicated two-query OpenAPI fixture helper**

Add a helper such as:

```go
func writeTwoQuerySpec(t *testing.T, port int) string
```

The operation must be:

```yaml
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
```

Keep the same HTTPS server base.

- [ ] **Step 3: Add `TestDockerImageRunsTwoQueryToolOverStdio`**

Use the same prebuilt `OASRELAY_IMAGE`.

Assert non-root image user and run the real container with:

- `OASRELAY_BEARER_TOKEN=container-secret`;
- the generated CA;
- read-only spec and CA mounts;
- `serve --operation-id listCustomers`.

List tools and assert exactly `listCustomers`.

- [ ] **Step 4: Call with only the required query**

Call:

```json
{"limit": 25}
```

Require exact upstream:

```text
GET /api/customers?limit=25
```

and the existing Bearer header.

Verify structured response and secret non-disclosure.

- [ ] **Step 5: Call with both queries**

Call:

```json
{"limit": 25, "cursor": "next page"}
```

Require exact upstream:

```text
GET /api/customers?limit=25&cursor=next+page
```

This exact order is part of the contract.

Require the same Bearer header, structured response, and secret non-disclosure.

Require exactly two upstream calls for this test.

- [ ] **Step 6: Run Docker acceptance**

Run:

```bash
./scripts/test-docker.sh
```

Expected: PASS for both the pre-existing path+optional-query test and the new two-query test.

- [ ] **Step 7: Run full Go suite**

Run:

```bash
go test ./...
go vet ./...
```

Expected: PASS.

- [ ] **Step 8: Commit Docker evidence**

```bash
git add internal/containertest/docker_stdio_test.go
git commit -m "test: cover two query parameters in Docker acceptance"
```

- [ ] **Step 9: Task self-review**

Confirm the new acceptance did not replace the old one and still proves:

- one real built image;
- non-root runtime;
- stdio;
- HTTPS trust;
- Bearer forwarding;
- secret non-disclosure;
- required+optional omission/presence;
- declaration-order query binding.

---

### Task 5: Align README and Run Final Verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- Documents only the shipped two-query contract.
- Does not advertise arbitrary multiplicity, path+two-query, generic parameters, security schemes, or multiple tools.

- [ ] **Step 1: Update the current-scope summary**

Document the exact supported cardinality:

- zero parameters;
- one primitive query parameter;
- two primitive query parameters when there is no path parameter;
- one required path parameter;
- one required path + one primitive query parameter;
- at most two operation-level parameters total.

- [ ] **Step 2: Add a two-query example**

Use `limit` required and `cursor` optional.

Document:

```json
{"limit": 25}
```

=> `?limit=25`.

And:

```json
{"limit": 25, "cursor": "next page"}
```

=> `?limit=25&cursor=next+page`.

State that order follows OpenAPI declaration order.

- [ ] **Step 3: Document omission and non-goals**

Explicitly state:

- each query can be required or optional;
- explicit `null` is invalid;
- defaults remain unsupported;
- two optional queries may both be omitted;
- path + two query is unsupported;
- three query is unsupported;
- generic parameter multiplicity is not supported.

Keep all existing security/multiple-tool non-goals.

- [ ] **Step 4: Run formatting and module hygiene**

Run:

```bash
gofmt -w internal/openapi/select.go internal/openapi/query_parameter_test.go internal/openapi/path_parameter_test.go internal/openapi/combined_parameter_test.go cmd/oasrelay/query_parameter_test.go cmd/oasrelay/path_parameter_test.go cmd/oasrelay/combined_parameter_test.go internal/mcpserver/server.go internal/mcpserver/query_parameter_test.go internal/mcpserver/combined_parameter_test.go internal/mcpserver/base_query_test.go internal/containertest/docker_stdio_test.go
go mod tidy
git diff --exit-code -- go.mod go.sum
```

Expected: no dependency drift.

- [ ] **Step 5: Run complete Go verification**

Run:

```bash
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

Expected: PASS and existing deterministic inspect output.

- [ ] **Step 6: Run final Docker acceptance**

Run:

```bash
./scripts/test-docker.sh
```

Expected: PASS.

- [ ] **Step 7: Commit documentation**

```bash
git add README.md
git commit -m "docs: document two query parameters"
```

- [ ] **Step 8: Perform full scope self-review**

Check the final branch against the spec:

- total operation parameter cap is still two;
- no three-query support;
- no path+two-query support;
- no two-path support;
- no optional path support;
- no generic `Parameter` IR;
- no header/cookie support;
- no schema/default expansion;
- no auth/security-scheme changes;
- no multiple-tool changes;
- one-query and path+one-query regressions remain green;
- raw-query preservation remains exact;
- validation remains pre-network;
- query output order follows selected declaration order.

- [ ] **Step 9: Record final evidence**

Record:

- branch name;
- final commit SHA;
- Task 1 migration RED/GREEN;
- Task 2 selector RED/GREEN;
- Task 3 runtime RED/GREEN;
- Task 4 Docker acceptance result;
- Task 5 final `go test ./...`, `go vet ./...`, inspect, and Docker results;
- final scope self-review.

Do not claim completion without fresh final-tree evidence.

---

## Plan Self-Review

### Spec coverage

PASS. Every spec section maps to a task:

- model migration => Task 1;
- two-query selection/cardinality/order => Task 2;
- MCP schema/binding/omission/null/raw-query/validation => Task 3;
- Docker evidence => Task 4;
- docs/final verification => Task 5.

### Step scan

PASS. Production behavior is never added before a failing behavior test:

- Task 1 uses compile RED to migrate the project-owned interface;
- Task 2 adds selector RED before selection support;
- Task 3 adds schema/runtime RED before executable two-query support;
- Task 4 is acceptance-only after the behavior exists;
- Task 5 is documentation/verification only.

### Type consistency

PASS.

Canonical final interfaces:

```go
type QueryParameter struct {
    Name     string
    Type     string
    Optional bool
}

type SelectedOperation struct {
    OperationID     string
    Method          string
    Path            string
    Summary         string
    Description     string
    Endpoint        string
    QueryParameters []QueryParameter
    PathParameter   *PathParameter
}
```

Runtime binder:

```go
func bindQueryParameters(
    endpoint string,
    parameters []oasopenapi.QueryParameter,
    input map[string]json.RawMessage,
) (string, error)
```

No task uses the removed `SelectedOperation.QueryParameter` field after Task 1.

### Review Focus coverage

PASS.

- optional-first + required-second => Tasks 2 and 3;
- explicit empty string => Task 3;
- duplicate query names => Tasks 2 and 3;
- malformed path + two queries => Task 3;
- both optional omitted with raw query => Task 3.

### Proportion

PASS. The plan preserves the spec's five-layer implementation path without transcribing function bodies. The cross-cutting model migration is isolated first because the field change otherwise makes later task boundaries compile-broken.

## Execution Boundary

This plan does not authorize implementation by itself.

No production code or test implementation should begin until the human reviewer approves this plan and confirms the execution method.

For this repository/harness, **Native / executing-plans** is the practical execution method because the tasks share one model migration interface and no subagent execution facility has been available in prior work.

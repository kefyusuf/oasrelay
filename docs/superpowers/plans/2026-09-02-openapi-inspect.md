# OpenAPI Inspect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Implement each task test-first and keep Issue #1 as the complete scope boundary.

**Goal:** Build `oasrelay inspect <local-spec-path>` so a local OpenAPI document can be validated and its `GET` operations listed deterministically.

**Architecture:** `internal/openapi` owns local loading, version checks, validation, and discovery. `cmd/oasrelay` owns argument handling, report formatting, diagnostics, and exit codes. The CLI consumes only project-owned result types.

**Tech Stack:** Go 1.25+, Go standard library, `github.com/getkin/kin-openapi` v0.149.0.

**Spec:** `docs/superpowers/specs/2026-09-02-openapi-inspect-design.md`

## Global constraints

- Implement only the `inspect` command in Issue #1.
- Read one local YAML or JSON file supplied as a positional argument.
- Accept OpenAPI 3.x versions supported by `kin-openapi` v0.149.0; reject other versions.
- Allow internal document references but keep external file and URL `$ref` resolution disabled.
- Discover only `GET` operations; do not execute requests.
- Sort operations by path, then `operationId`.
- Preserve a missing `operationId` as an empty string and render its warning in the CLI layer.
- Use standard-library CLI parsing; do not add a command framework.
- Return concise errors without stack traces.
- Do not add MCP, authentication, Docker, configuration, persistence, UI, policy, generation, or SaaS code.

## File map

- `go.mod`, `go.sum`: module and dependency metadata.
- `internal/openapi/inspect.go`: inspection result types and `InspectFile`.
- `internal/openapi/inspect_test.go`: loading, validation, ordering, reference, and error tests.
- `cmd/oasrelay/main.go`: entry point, dispatch, formatting, and exit codes.
- `cmd/oasrelay/main_test.go`: CLI behavior tests without subprocesses.
- `testdata/*.yaml`, `testdata/*.json`: valid and invalid fixtures.
- `README.md`: only the capability that actually exists.

---

### Task 1: Define inspection behavior with failing tests

**Files:**
- Create: `go.mod`
- Create: `internal/openapi/inspect_test.go`
- Create: `testdata/customer-api.yaml`
- Create: `testdata/customer-api.json`
- Create: `testdata/missing-operation-id.yaml`
- Create: `testdata/invalid-openapi.yaml`
- Create: `testdata/unsupported-version.yaml`

**Interfaces produced by the later implementation:**

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

func InspectFile(path string) (Inspection, error)
```

- [ ] Add `go.mod` with module `github.com/kefyusuf/oasrelay`, Go 1.25, and `kin-openapi` v0.149.0.
- [ ] Add a YAML fixture containing two `GET` operations, one ignored `POST`, a first server URL, and an internal `#/components/...` reference.
- [ ] Add a valid JSON OpenAPI 3.1 fixture.
- [ ] Add fixtures for missing `operationId`, invalid OpenAPI 3 validation, and unsupported version.
- [ ] Write tests for YAML metadata, JSON loading, deterministic sorting, ignored non-GET operations, internal references, missing `operationId`, absent server, invalid documents, unsupported versions, unreadable paths, and rejected external references.
- [ ] Run `go test ./internal/openapi` and verify RED because `InspectFile`, `Inspection`, and `Operation` do not exist.

### Task 2: Implement the inspection package

**Files:**
- Create: `internal/openapi/inspect.go`
- Create: `go.sum`

**Implementation contract:**

```go
func InspectFile(path string) (Inspection, error) {
    loader := openapi3.NewLoader()
    loader.IsExternalRefsAllowed = false
    // LoadFromFile, reject unsupported version, Validate,
    // extract metadata and GET operations, then sort.
}
```

- [ ] Implement only the result types and `InspectFile` required by Task 1.
- [ ] Wrap load errors with `load document` and validation errors with `validate document`.
- [ ] Use `document.OpenAPIMajorMinor()` to reject unsupported versions before validation.
- [ ] Resolve internal references through the loader while leaving external references disabled.
- [ ] Run `gofmt`, `go mod tidy`, and `go test ./internal/openapi`; verify GREEN.
- [ ] Commit as `feat: inspect local OpenAPI documents`.

### Task 3: Define CLI behavior with failing tests

**Files:**
- Create: `cmd/oasrelay/main_test.go`

**Interfaces produced by the later implementation:**

```go
func run(args []string, stdout, stderr io.Writer) int
func writeInspection(w io.Writer, inspection openapi.Inspection)
```

- [ ] Test exact deterministic success output.
- [ ] Test missing `operationId` rendering.
- [ ] Test no arguments, missing inspect path, excess arguments, and unknown commands as exit code `2`.
- [ ] Test invalid input as exit code `1`, with the path and operation context on standard error.
- [ ] Run `go test ./cmd/oasrelay` and verify RED because `run` does not exist.

### Task 4: Implement the CLI

**Files:**
- Create: `cmd/oasrelay/main.go`

**CLI contract:**

```text
oasrelay inspect <local-spec-path>
```

- [ ] Implement `main`, `run`, and `writeInspection` with the standard library.
- [ ] Use exit code `2` for usage errors and `1` for operational errors.
- [ ] Print the server line only when a server exists.
- [ ] Render `<missing operationId>` and its future-MCP warning without changing the package result.
- [ ] Run `gofmt` and `go test ./...`; verify GREEN.
- [ ] Run `go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml` and confirm only the two `GET` operations appear.
- [ ] Commit as `feat: add OpenAPI inspect command`.

### Task 5: Document and verify the completed slice

**Files:**
- Create: `README.md`

- [ ] State that the current version only inspects local OpenAPI 3.x documents and does not run an MCP server.
- [ ] Document Go 1.25+, the inspect command, tests, implemented behavior, and explicit non-goals.
- [ ] Run:

```bash
gofmt -w cmd/oasrelay/main.go cmd/oasrelay/main_test.go internal/openapi/inspect.go internal/openapi/inspect_test.go
go test ./...
go vet ./...
go run ./cmd/oasrelay inspect ./testdata/customer-api.yaml
```

- [ ] Review the diff and reject any MCP, HTTP client, authentication, Docker, configuration, persistence, UI, policy, generation, or SaaS path.
- [ ] Commit as `docs: explain initial inspect workflow`.

## Definition of done

Issue #1 is complete only when the full verification sequence passes, the CLI output matches the fixtures, external references remain blocked, the diff contains only the stated slice, and the work is submitted through a pull request without merging it automatically.
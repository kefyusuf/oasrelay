# OpenAPI Inspect Implementation Plan

**Status:** Completed in pull request #2; not merged  
**Goal:** Build `oasrelay inspect <local-spec-path>` so a local OpenAPI document can be validated and its `GET` operations listed deterministically.

**Architecture:** `internal/openapi` owns local loading, version checks, validation, and discovery. `cmd/oasrelay` owns argument handling, report formatting, diagnostics, and exit codes. The CLI consumes only project-owned result types.

**Tech stack:** Go 1.25+, Go standard library, `github.com/getkin/kin-openapi` v0.149.0.  
**Design:** `docs/superpowers/specs/2026-09-02-openapi-inspect-design.md`

## Locked scope

- Read one local YAML or JSON file supplied as a positional argument.
- Accept OpenAPI 3.x versions supported by `kin-openapi` v0.149.0.
- Allow internal document references; reject external file and URL `$ref` values.
- Discover only `GET` operations; never execute requests.
- Sort operations by path, then `operationId`.
- Preserve missing `operationId` values and render their warnings in the CLI layer.
- Use the standard library for CLI dispatch.
- Do not add MCP, authentication, Docker, configuration, persistence, UI, policy, generation, or SaaS code.

## Completed tasks

### 1. Define inspection behavior

- [x] Declare the Go module and pin `kin-openapi` v0.149.0.
- [x] Add YAML and JSON OpenAPI fixtures.
- [x] Cover deterministic ordering and ignored non-GET operations.
- [x] Cover internal references, missing `operationId`, and absent servers.
- [x] Cover malformed input, invalid documents, unsupported versions, unreadable paths, and external references.
- [x] Record the initial RED test run before production symbols existed.

### 2. Implement the inspection package

- [x] Add project-owned `Inspection` and `Operation` result types.
- [x] Implement `InspectFile` using a loader with external references disabled.
- [x] Validate the document before extracting metadata and operations.
- [x] Wrap load and validation errors with stable operation context.
- [x] Run package tests successfully.

### 3. Implement the CLI

- [x] Test exact report output, missing IDs, usage errors, and operational errors.
- [x] Implement `main`, `run`, and `writeInspection` without a CLI framework.
- [x] Use exit code `2` for usage failures and `1` for document failures.
- [x] Verify the command lists only the two expected `GET` operations.

### 4. Document and verify the slice

- [x] Document only the implemented `inspect` capability and explicit non-goals.
- [x] Add CI checks for tidy module files, formatting, tests, vet, and a CLI smoke run.
- [x] Review the full diff for scope expansion and reject unrelated subsystems.
- [x] Submit the implementation through pull request #2 without merging it automatically.

## Definition of done

The slice is complete when the CI verification is green, output matches the fixtures, external references remain blocked, the diff remains within Issue #1, and pull request #2 is ready for human merge review.

# Inspect selection diagnostics

## Scope

Continue from merged string-enum PR #22 (`3a0eb87`). Add opt-in `inspect --diagnostics <local-spec-path>` to explain which discovered GET operations the existing selector accepts and why it rejects others. Keep the default inspect report and exit semantics, one GET/stdin-stdout runtime tool, and all existing parameter/authentication boundaries.

## Contract

- Load and validate the local document once, with external references blocked.
- Discover and sort GET operations exactly as default inspect does.
- Run the existing `selectGETOperation` against the same document and operation. Do not duplicate selection rules or load the file once per operation.
- Show `Selectable: yes` or `Selectable: no` and the first selector rejection reason. Missing operation IDs are rejected by the existing name validation.
- Unsupported operations remain in the report; they do not change a valid document's exit code of zero. Document errors return one; invalid usage returns two.
- Diagnostics mean static selection only. Do not start MCP, perform HTTP requests, or inspect credentials. A selectable operation is not proof of upstream availability or authentication readiness.
- `--diagnostics=false` produces the default report. Flags precede the positional path, matching the established CLI convention.

## Verification

Use RED tests for backend diagnostics and CLI opt-in behavior, then implement the smallest shared inspection helper. Cover mixed supported/rejected GETs, deterministic discovery, operation server precedence, inherited/overridden enum schemas, missing IDs, invalid documents, external references, CLI usage, and absence of runtime delegation or credential handling. Keep existing exact-output tests for default inspect unchanged. Run standard Go checks and existing real Docker CI acceptance before declaring the PR ready.

## Continuation

Implementation and publication evidence will be recorded in the PR. This increment does not merge or release itself and does not add further runtime/schema features.

## Local evidence

Observed RED for the missing diagnostics API/model and for CLI opt-in usage. Fresh `go test -count=1 ./...`, `go vet ./...`, module tidy consistency, and diff checks passed after implementation. The customer fixture produced per-operation `Selectable: yes` lines and the static-selection disclaimer. Independent source review found no actionable correctness or regression issue. Default report tests remain unchanged; documentation explains the flag syntax. Actual Docker execution is unavailable on this host; final-head Linux/Docker CI evidence is maintained in the PR checks and validation section.

## Publication checkpoint

[PR #23](https://github.com/kefyusuf/oasrelay/pull/23) on `feat/inspect-diagnostics` contains test-contract commit `5c0f669` and implementation commit `f5ef676`, based on main `3a0eb87`. Verify final-head CI before treating the PR as ready. Keep continuation limited to this diagnostics increment until review/merge; further runtime capabilities require a separate scope decision.

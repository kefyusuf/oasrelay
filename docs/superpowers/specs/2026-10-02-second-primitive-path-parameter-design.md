# Second Primitive Path Parameter

## Scope

Support exactly two required primitive operation-level path parameters for one selected GET operation. The total operation-level parameter cap stays two. The runtime still exposes exactly one MCP tool over stdio.

Example: `/customers/{customerId}/orders/{orderId}` accepts both required input fields. Two paths plus any query, three paths, optional paths, headers, cookies, inherited path-item parameters, request bodies, and non-GET methods remain unsupported.

Existing zero-parameter, one/two-query, one-path, and one-path-plus-one-query behavior is preserved. Schemas remain plain `string`, `integer`, `number`, or `boolean`; defaults, constraints, enums, arrays, objects, and custom serialization remain unsupported.

## Model and Selection

Replace `SelectedOperation.PathParameter *PathParameter` with `PathParameters []PathParameter`. Preserve the `PathParameter` type and the existing bounded query slice; do not introduce a generic parameter model.

Store path contracts in OpenAPI declaration order. Each name must be distinct, each parameter must remain required and use default simple serialization, and each declared name must match exactly one route placeholder. After removing all declared placeholders, no undeclared or malformed placeholders may remain. Declaration order may differ from route order.

The MCP constructor defensively enforces at most two total path/query contracts and unique input names, including malformed internal models.

## Binding

The closed MCP object exposes all path properties as required. Missing fields, null, wrong primitive types, and unknown fields must fail before any upstream request.

Validate all path values and the original template before replacement. Construct decoded and escaped paths with one-pass, non-recursive replacement of the original placeholders. Never re-scan inserted caller data as a template. For example, customerId `{orderId}` and orderId `42` produce `/customers/%7BorderId%7D/orders/42`.

Encode each value as one path segment. Preserve escaped slashes, percent signs, Unicode, literal dot segments, integer canonicalization, server path escaping, and raw server query text. Existing Bearer/HTTPS/redirect behavior, timeout, response-size limits, and Docker non-root configuration are unchanged.

## Verification and Execution

Baseline: `main` commit `0939ea6`, with Go and Docker CI already green after PR #19.

Execute on `feat/second-primitive-path-parameter` using test-first changes:

1. Migrate existing tests to the plural path model, prove compilation RED, then migrate production without enabling a second path. Verify existing behavior remains green.
2. Add selector and CLI tests for two required paths, declaration order, primitive types, and malformed/cardinality rejection. Prove behavioral RED, implement selection, then prove GREEN.
3. Add MCP schema and execution tests, including placeholder-shaped caller data, segment escaping, invalid input with zero upstream calls, raw-query preservation, and malformed models. Prove behavioral RED, enable schema and binding atomically, then prove GREEN.
4. Add real Docker stdio acceptance while retaining the existing path-plus-query and two-query tests. Update README.
5. Run required Go/module/format checks, independent source review, and final Docker CI. Commit and create a reviewable PR; do not merge as part of this feature implementation.

The current host has no Docker executable. Real container verification uses the existing GitHub Actions Docker job; a container-tagged compile check is not container execution.

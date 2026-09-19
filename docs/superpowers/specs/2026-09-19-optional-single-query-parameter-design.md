# Optional Single Query Parameter Design

## Status

Approved product/scope direction. This document persists the bounded design before implementation.

Implementation has not started.

## Goal

Extend the existing single-tool GET runtime so its one supported operation-level query parameter may be either required or optional, while preserving the existing cardinality and all current safety boundaries.

Supported cardinality remains:

- zero or one operation-level path parameter;
- zero or one operation-level query parameter;
- at most two operation-level parameters total;
- exactly one exposed MCP tool.

The new capability is optionality, not multiplicity.

## Supported Shapes

The runtime supports these operation shapes:

1. no parameters;
2. one required primitive query parameter;
3. one optional primitive query parameter;
4. one required primitive path parameter;
5. one required primitive path parameter plus one required primitive query parameter;
6. one required primitive path parameter plus one optional primitive query parameter.

Optional path parameters remain unsupported. OpenAPI path parameters must remain required.

## Query Parameter Contract

A supported query parameter must:

- be declared directly on the selected operation;
- use `in: query`;
- use the default OpenAPI query serialization already required by the runtime;
- use a plain primitive schema of type `string`, `integer`, `number`, or `boolean`;
- remain free of arrays, objects, unions, nullable, enum, format, bounds, patterns, defaults, or other schema constraints currently rejected by `isPlainPrimitiveSchema`.

The OpenAPI `required` flag may be either `true` or `false`. When omitted, OpenAPI's default false value is treated as optional.

## Project-Owned Model

Keep the existing bounded model. Do not introduce a generic parameter collection.

Use:

```go
type QueryParameter struct {
    Name     string
    Type     string
    Optional bool
}
```

`Optional` is intentionally chosen instead of `Required` so the Go zero value preserves the existing required behavior for current internal constructors and tests.

Semantics:

- `Optional == false`: caller must supply the query argument;
- `Optional == true`: caller may omit the query argument.

The existing `PathParameter` model remains unchanged.

## MCP Input Schema

The MCP input remains a closed object with:

```json
{
  "type": "object",
  "properties": {},
  "additionalProperties": false
}
```

Required query parameters remain present in the schema's `required` list.

Optional query parameters remain present under `properties`, but are omitted from `required`.

For one required path plus one optional query parameter, only the path parameter appears in `required`.

Examples:

Optional query only:

```json
{
  "type": "object",
  "properties": {
    "limit": {"type": "integer"}
  },
  "additionalProperties": false
}
```

Required path plus optional query:

```json
{
  "type": "object",
  "properties": {
    "customerId": {"type": "string"},
    "limit": {"type": "integer"}
  },
  "required": ["customerId"],
  "additionalProperties": false
}
```

## Omitted Versus Null

Optionality and nullability are separate.

For an optional integer query parameter:

- `{}` is valid and sends no operation query parameter;
- `{"limit": 25}` is valid and sends `limit=25`;
- `{"limit": null}` is invalid because nullable schemas remain unsupported;
- a wrong primitive type is invalid;
- an unknown field is invalid.

The same distinction applies to other primitive types.

## Binding Semantics

When the optional query argument is omitted:

- do not add any query pair;
- return the endpoint unchanged after any required path binding;
- preserve an existing raw server query byte-for-byte;
- do not parse and re-encode the existing raw query.

When the optional query argument is supplied:

- validate it with the existing primitive conversion rules;
- preserve canonical integer serialization;
- append the encoded query pair using the existing raw-query preservation behavior.

For a required path plus optional query:

1. validate the complete input shape;
2. require and bind the path parameter;
3. if the optional query field is present, validate and append it;
4. otherwise preserve the path-bound endpoint without adding the operation query.

No upstream HTTP call may occur before argument validation succeeds.

## Empty String

An explicitly supplied empty string remains distinct from omission.

For a supported plain string query parameter:

```json
{"filter": ""}
```

continues to serialize as an explicitly supplied empty value under the runtime's existing primitive behavior.

This slice does not add new `allowEmptyValue` semantics.

## Schema Defaults

OpenAPI schema defaults remain unsupported.

A query parameter such as:

```yaml
- name: limit
  in: query
  required: false
  schema:
    type: integer
    default: 25
```

continues to be rejected by the existing plain-primitive schema policy.

OASRelay must not inject `?limit=25` when the caller omits the argument, and it must not silently accept-and-ignore the unsupported default.

## Existing Invariants Preserved

This slice does not change:

- GET-only execution;
- exactly one MCP tool;
- operation-level parameters only;
- zero-or-one path parameter;
- zero-or-one query parameter;
- path parameters remain required;
- path placeholder matching and single-segment escaping;
- default query serialization requirements;
- canonical integer serialization;
- existing raw server query preservation;
- closed MCP input objects;
- argument validation before upstream execution;
- request timeout and response size limits;
- process-level Bearer authentication;
- HTTPS requirement for non-empty Bearer credentials;
- same-origin redirect credential policy;
- stdio transport;
- Docker non-root runtime.

## Explicit Non-Goals

Not included:

- multiple query parameters;
- multiple path parameters;
- optional path parameters;
- header parameters;
- cookie parameters;
- path-item-level parameter inheritance;
- arrays or object parameters;
- enum, nullable, unions, schema constraints, or schema defaults;
- custom parameter serialization;
- OpenAPI `securitySchemes` processing;
- request bodies;
- non-GET methods;
- multiple MCP tools;
- Streamable HTTP;
- generic parameter IR;
- configuration or policy systems.

## Verification Contract

Implementation must prove:

- selector accepts optional primitive query parameters for all four supported primitive types;
- selector records optionality in the project-owned model;
- omitted `required` and explicit `required: false` behave equivalently;
- required query selection remains unchanged;
- optional query-only MCP schema omits `required`;
- required path plus optional query MCP schema requires only the path field;
- omitted optional query succeeds with zero added operation query pairs;
- supplied optional query binds correctly;
- explicit null fails before upstream execution;
- wrong primitive type fails before upstream execution;
- unknown input fields fail before upstream execution;
- required path remains mandatory when paired with an optional query;
- existing raw server query text is preserved exactly when the optional query is omitted;
- existing raw server query text is preserved and the operation query is appended when supplied;
- canonical integer serialization remains unchanged;
- parameterless, required-query, required-path, required path+query, Bearer, redirect, and Docker regressions remain green;
- Docker acceptance proves both omitted and supplied optional-query calls without weakening TLS, Bearer, secret non-disclosure, and non-root assertions.

## Design Self-Review

- Scope alignment: PASS — optionality only; multiplicity remains fixed.
- Dependency direction: PASS — no new package or subsystem.
- Backward safety: PASS — `Optional bool` preserves required behavior at the Go zero value.
- OpenAPI semantics: PASS — omission is distinct from null.
- YAGNI: PASS — no generic parameter IR or broader schema system.
- Security: PASS — authentication and redirect policies are untouched.
- Verification: PASS — selector, MCP schema, binding, and container acceptance are all covered.

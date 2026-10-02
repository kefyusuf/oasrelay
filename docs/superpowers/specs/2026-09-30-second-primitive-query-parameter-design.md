# Second Primitive Query Parameter Design

## Status

Implemented on branch `feat/second-primitive-query-parameter` in [PR #19](https://github.com/kefyusuf/oasrelay/pull/19). The PR is open and ready for review; the feature has not been merged into `main`.

Implementation and acceptance checkpoint: `1d1d277cc506710a1ca0f4c9f3b20dd0d7be817f`. Both Go verification and Docker acceptance passed in [CI run 36975916031](https://github.com/kefyusuf/oasrelay/actions/runs/36975916031). See the implementation plan's execution checkpoint for continuation state. The context below describes the baseline before this feature.

## Context

OASRelay currently exposes exactly one selected OpenAPI `GET` operation as one MCP tool over stdio.

The current operation-level parameter envelope is deliberately narrow:

- zero parameters;
- one primitive query parameter, required or optional;
- one required primitive path parameter;
- one required primitive path parameter plus one primitive query parameter, required or optional;
- at most two operation-level parameters total.

The next product question is not arbitrary parameter multiplicity. It is whether one common missing GET shape can be supported without expanding the total parameter envelope or introducing a generic parameter engine.

## Goal

Support exactly two primitive operation-level query parameters when there is no path parameter, while preserving the existing maximum of two operation-level parameters total.

The new capability is:

> second primitive query parameter, not arbitrary multiple parameters.

Representative shapes include:

- `?limit=25&offset=50`;
- `?page=2&perPage=50`;
- `?filter=active&sort=name`.

## Supported Shapes

The runtime supports these operation shapes:

1. no parameters;
2. one required primitive query parameter;
3. one optional primitive query parameter;
4. two primitive query parameters;
5. one required primitive path parameter;
6. one required primitive path parameter plus one primitive query parameter.

For two query parameters, each query may independently be required or optional.

Supported two-query requiredness combinations:

- required + required;
- required + optional;
- optional + required;
- optional + optional.

The total operation-level parameter cap remains exactly two.

Therefore these remain unsupported:

- one path plus two query parameters;
- three query parameters;
- two path parameters;
- any other three-parameter operation shape.

## Product Rationale

This slice increases compatibility with ordinary read-only REST APIs without changing OASRelay's one-operation, one-tool, least-authority runtime model.

It is preferred over the other reassessment candidates for this milestone because:

- OpenAPI `securitySchemes` would create a broader credential-selection and secret-resolution trust boundary while process-level Bearer already covers the current narrow authentication path.
- Multiple MCP tools would change the runtime composition model, CLI selection contract, tool registration cardinality, and least-authority boundary.
- A second primitive query parameter extends the selector/schema/binding path that already exists, with no new transport, credential, or execution subsystem.

## Project-Owned Model

The current single-query pointer is no longer sufficient:

```go
type SelectedOperation struct {
    QueryParameter *QueryParameter
    PathParameter  *PathParameter
}
```

Replace only the query cardinality representation:

```go
type SelectedOperation struct {
    QueryParameters []QueryParameter
    PathParameter   *PathParameter
}
```

The query slice has a strict semantic cardinality of `0..2`.

The existing query contract remains:

```go
type QueryParameter struct {
    Name     string
    Type     string
    Optional bool
}
```

The existing path contract remains unchanged.

This is intentionally not a generic parameter IR. Do not introduce:

```go
type Parameter struct {
    In       string
    Required bool
    Style    string
    Schema   ...
}
```

and do not replace the bounded path/query model with a generic `[]Parameter` collection.

### Why a slice instead of a second pointer

Do not add fields such as:

```go
QueryParameter       *QueryParameter
SecondQueryParameter *QueryParameter
```

That would encode ordinal positions into the model and make requiredness, schema generation, validation, and future maintenance more error-prone.

A bounded `[]QueryParameter` with an enforced `0..2` cardinality expresses the new capability directly without generalizing parameter locations or schema semantics.

## Selection Contract

The selector must preserve the current top-level cap:

```text
len(operation.Parameters) <= 2
```

This cap must not be increased.

Selection behavior:

- zero operation parameters => existing parameterless behavior;
- one query => existing behavior;
- two query => new supported shape;
- one path => existing behavior;
- one path + one query => existing behavior;
- two path => rejected;
- one path + two query => rejected by the unchanged total cap;
- three query => rejected by the unchanged total cap;
- header/cookie => rejected.

The selector should collect at most:

- two query parameters;
- one path parameter.

If a path parameter is present, the total cap guarantees there can be at most one query parameter.

All MCP input names must be unique across the selected operation. Two query parameters with the same name must be rejected defensively even if upstream OpenAPI validation would normally catch the shape.

## Query Parameter Contract

Every supported query parameter must continue to:

- be declared directly on the selected operation;
- use `in: query`;
- use default query serialization;
- use a plain primitive schema of type `string`, `integer`, `number`, or `boolean`;
- remain free of arrays, objects, enum, nullable, unions, defaults, format, bounds, patterns, conditional schemas, or other constraints already rejected by `isPlainPrimitiveSchema`.

Each query independently maps OpenAPI requiredness to:

```text
Optional = !parameter.Required
```

An omitted OpenAPI `required` field is therefore optional, as today.

## Declaration Order and Determinism

The query slice must preserve OpenAPI operation-level declaration order.

That order is the canonical runtime binding order for two query parameters.

For example, if the spec declares:

```yaml
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

and the server URL already contains:

```text
?token=a;b
```

then this input:

```json
{
  "limit": 25,
  "cursor": "next page"
}
```

must produce:

```text
?token=a;b&limit=25&cursor=next+page
```

Do not encode both operation query parameters by placing them into one `url.Values` and calling `Encode()`, because that sorts parameter names lexicographically and would make output order depend on key names rather than the selected OpenAPI contract.

Encode and append each selected query pair in declaration order using the existing per-parameter primitive serialization rules.

## MCP Input Schema

The MCP input remains a closed object:

```json
{
  "type": "object",
  "properties": {},
  "additionalProperties": false
}
```

For two query parameters:

- both appear under `properties`;
- only non-optional parameters appear in `required`;
- the `required` list follows query declaration order;
- `additionalProperties` remains `false`.

Example: required `limit` plus optional `cursor`:

```json
{
  "type": "object",
  "properties": {
    "limit": {"type": "integer"},
    "cursor": {"type": "string"}
  },
  "required": ["limit"],
  "additionalProperties": false
}
```

For the existing path-plus-one-query shape:

- the required path remains first in the `required` list;
- a required query follows it;
- an optional query remains absent from `required`.

Do not alter existing path-plus-query schema semantics while migrating the query representation.

## Binding Semantics

Introduce a bounded query binding path that accepts zero, one, or two selected query contracts but rejects any model state outside that range.

The binding algorithm for query-only operations is:

1. validate that every supplied input field belongs to a selected query parameter;
2. validate that every non-optional query parameter is present;
3. reject explicit `null` for any supplied query parameter;
4. validate primitive type using the existing conversion rules;
5. if no selected optional query value is supplied and there are no required query values to bind, return the endpoint unchanged;
6. otherwise append each supplied query pair in selected declaration order.

The implementation may use a local helper such as:

```go
bindQueryParameters(
    endpoint string,
    parameters []oasopenapi.QueryParameter,
    input map[string]json.RawMessage,
) (string, error)
```

if it remains limited to the query contract.

Do not introduce a generic parameter-validation subsystem.

### Existing path-plus-query binding

The existing path-plus-one-query flow remains:

1. require and bind the path;
2. pass the one selected query parameter through the same bounded query binder;
3. append the query only when supplied if it is optional.

No path-plus-two-query behavior is introduced.

## Omitted Versus Null

Optionality and nullability remain separate per query parameter.

For optional `cursor`:

- omitted => no `cursor` query pair;
- `{"cursor": "next"}` => append `cursor=next`;
- `{"cursor": ""}` => append the explicitly supplied empty value;
- `{"cursor": null}` => error;
- wrong primitive type => error.

For two optional queries:

```json
{}
```

is valid and must leave the endpoint unchanged, including any existing raw server query.

## Existing Raw Query Preservation

Existing server query text must remain byte-for-byte preserved.

Given:

```text
https://example.test/customers?token=a;b&mode=raw%2Fvalue
```

two omitted optional operation queries must leave the endpoint unchanged.

If only `limit=25` is supplied:

```text
?token=a;b&mode=raw%2Fvalue&limit=25
```

If both `limit=25` and `cursor=next` are supplied in that declaration order:

```text
?token=a;b&mode=raw%2Fvalue&limit=25&cursor=next
```

Do not parse and re-encode the existing raw query merely because optional operation queries exist.

## Validation and Network Boundary

All argument validation must complete before `upstream.Get`.

Errors include:

- missing required query parameter;
- explicit `null`;
- wrong primitive type;
- unknown input field;
- duplicate selected input name;
- unsupported selected-operation model cardinality.

No validation error may produce an upstream HTTP request.

## Existing Invariants Preserved

This slice does not change:

- local OpenAPI loading;
- GET-only execution;
- exact `operationId` selection;
- exactly one MCP tool;
- stdio transport only;
- maximum two operation-level parameters total;
- maximum one path parameter;
- path parameters remain required;
- operation-level parameters only;
- path placeholder matching;
- single-segment path escaping;
- default query serialization;
- primitive type set;
- canonical integer serialization;
- omitted-versus-null behavior;
- raw server query preservation;
- closed MCP input objects;
- validation before network;
- fixed request timeout;
- fixed response size limit;
- process-level Bearer authentication;
- HTTPS requirement for Bearer;
- same-origin Bearer redirect policy;
- non-root Docker runtime.

## Docker Acceptance

Do not replace the existing required-path-plus-optional-query Docker evidence.

Preserve the current acceptance test proving:

- numeric user `65532:65532`;
- stdio MCP transport;
- read-only OpenAPI and CA mounts;
- HTTPS trust;
- Bearer forwarding;
- secret non-disclosure;
- path + optional query omission and presence.

Add bounded container evidence for the new two-query shape.

The new acceptance may use a second selected operation or a second container test, but it must not broaden runtime behavior.

It must prove at least:

1. two selected primitive query properties are exposed;
2. one required plus one optional query can be called with only the required field;
3. both can be supplied;
4. exact upstream query order matches declaration order;
5. existing TLS/Bearer/non-root/secret controls remain unchanged.

Avoid unrelated Docker test refactoring unless required to keep duplication manageable.

## Explicit Non-Goals

Not included:

- three or more query parameters;
- arbitrary query multiplicity;
- path plus two query parameters;
- two path parameters;
- optional path parameters;
- header parameters;
- cookie parameters;
- path-item-level parameter inheritance;
- arrays;
- objects;
- enum;
- nullable schemas;
- unions;
- schema constraints;
- schema defaults;
- custom parameter serialization;
- OpenAPI `securitySchemes`;
- OAuth/OIDC;
- API-key or Basic authentication;
- request bodies;
- non-GET methods;
- multiple MCP tools;
- Streamable HTTP;
- remote OpenAPI loading;
- base URL overrides;
- generic parameter IR;
- generic parameter collection framework;
- configuration or policy subsystems.

## Verification Contract

Implementation must prove:

### Selector and model

- one query remains supported;
- optional one-query behavior remains supported;
- two primitive query parameters are accepted;
- query declaration order is preserved in the model;
- required/optional metadata is preserved independently for both queries;
- all four requiredness combinations are covered;
- a third query is rejected;
- one path plus two queries is rejected;
- two path parameters remain rejected;
- duplicate query names are rejected;
- header/cookie and unsupported schemas remain rejected.

### MCP schema

- both query properties are exposed;
- required + required => both required;
- required + optional => only required query listed;
- optional + required => only required query listed;
- optional + optional => no `required` field;
- `additionalProperties: false` remains;
- existing path-plus-query schemas remain unchanged.

### Runtime binding

- two required queries bind in declaration order;
- required + optional works with optional omitted;
- required + optional works with optional supplied;
- optional + optional with both omitted returns the endpoint unchanged;
- optional + optional with one supplied appends only that pair;
- both optional supplied append in declaration order;
- explicit null fails pre-network;
- wrong primitive type fails pre-network;
- missing required query fails pre-network;
- unknown field fails pre-network;
- existing raw query is preserved exactly;
- canonical integer serialization remains unchanged for either query.

### Regressions

- parameterless behavior remains green;
- one required query remains green;
- one optional query remains green;
- required path remains green;
- required path + required query remains green;
- required path + optional query remains green;
- Bearer and redirect tests remain green;
- Docker path-plus-optional-query acceptance remains green;
- new two-query Docker acceptance is green.

## Design Self-Review

- Scope alignment: PASS — one new query slot only; total operation parameter cap stays two.
- Product value: PASS — covers common GET pagination/filter combinations without changing runtime cardinality of tools or requests.
- Least authority: PASS — still one explicitly selected operation and one MCP tool.
- Data model: PASS — query cardinality becomes a bounded slice; path remains a distinct contract.
- Determinism: PASS — declaration-order binding is explicit and does not rely on map ordering or multi-key `url.Values.Encode()`.
- Security: PASS — no credential, destination, transport, or authorization subsystem change.
- Backward safety: PASS — path and one-query semantics remain explicit verification requirements.
- YAGNI: PASS — no arbitrary multiplicity, generic parameter IR, or policy/configuration framework.
- Verification: PASS — selector, schema, runtime, raw-query, validation, regression, and Docker evidence are all required.

## Approval Boundary

This design approves only the written specification for a second primitive query parameter under the unchanged two-parameter total cap.

It does not approve:

- an implementation plan;
- production code;
- a PR;
- security-scheme work;
- multiple-tool work;
- any broader parameter generalization.

The next step after human review of this document is an implementation plan, not implementation itself.

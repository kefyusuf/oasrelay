# Path Item Parameter Inheritance

## Contract

Select one GET operation with up to two effective primitive path/query parameters, declared on the path item, the operation, or both. Keep the existing single MCP tool, stdio transport, binding, authentication, and HTTP limits.

Merge by the exact pair `(name, in)`. An operation-level definition replaces the complete inherited definition with the same identity; it cannot remove other inherited parameters. Validate the document first and reject unresolved or duplicate entries within either source. Internal references remain supported; external references remain blocked.

Effective order starts with path-item declaration order. Overrides retain their inherited slot; operation-only definitions append in operation declaration order. This is OASRelay's deterministic serialization policy, not a specification requirement. Apply the total cap of two after merging, then reuse existing type, serialization, placeholder, and unique MCP input-name checks. Same names in different locations are distinct OpenAPI identities but remain unsupported as colliding MCP input names.

OpenAPI authority: [Path Item and Operation parameter rules](https://spec.openapis.org/oas/v3.0.3.html#path-item-object).

## Verification

- Observe selector, CLI, and spec-to-MCP integration tests failing before implementation.
- Cover inherited query/path combinations, complete overrides, stable ordering, effective cardinality, references, unsupported schemas/locations, and source-model integrity.
- Verify inherited required paths and overridden optional integer queries through MCP schema, omission/presence, escaping, canonical serialization, and invalid-input rejection before network access.
- Add real Docker stdio acceptance using the same inherited/overridden fixture, preserving existing acceptance cases.
- Run module, formatting, full Go, vet, inspect, container compilation, independent source review, and exact-head GitHub Docker CI.
- Submit a reviewable PR; merging the new feature is a separate step.

## Baseline

Branch: `feat/path-item-parameter-inheritance`, based on `main` commit `87a0670e3d1f71957259039e0550806c896d694c` (merged PR #20). No runtime model or new dependency is required.

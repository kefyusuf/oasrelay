# Published OpenAPI specimen provenance

- Source repository: https://github.com/swagger-api/swagger-petstore
- Source file: `src/main/resources/openapi.yaml`
- Pinned upstream revision: `d57941e8fe959e508796b27469b1e8bba73392dc`
- Source at that revision: https://github.com/swagger-api/swagger-petstore/blob/d57941e8fe959e508796b27469b1e8bba73392dc/src/main/resources/openapi.yaml
- Local copy: `testdata/petstore-openapi.yaml`
- Upstream license: Apache License 2.0 (copy at `testdata/PETSTORE-LICENSE.txt`)

Purpose: exercise the existing local-document inspection and one-tool GET stdio
runtime against a *published, independently maintained* OpenAPI specimen,
without introducing new runtime capabilities.

The Docker acceptance test uses the pinned document with **only** its `servers`
URL replaced by an ephemeral, local TLS fixture. It checks the same document
through `inspect --diagnostics` (offline, Docker network disabled) and
`serve --operation-id getUserByName` (two local fixture GET calls). It also
checks that unsupported GETs are reported as unselectable. **CI never calls
the public Petstore API or requires public Petstore credentials.**

The specimen includes OpenAPI security declarations, which OASRelay does not
interpret. `Selectable: yes` is *static shape acceptance only*, not an
authentication, authorization, or live-endpoint readiness claim.

No source-code changes, new authentication modes, published deployments,
multiple-tool support, or production external requests are part of this test.

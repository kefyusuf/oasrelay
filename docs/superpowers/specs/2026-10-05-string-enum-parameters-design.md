# String enum parameters

## Approved scope

Continue from merged inheritance PR #21 (`1d1fab6`) with string enums on the existing supported path/query parameters. Keep one GET operation, one stdio tool, default serialization, and at most two effective parameters. No additional schema constraints, authentication methods, or transport features.

## Contract

- Project-owned path/query metadata stores `Enum []string`; nil means absent.
- An explicit enum must contain at least one unique string and use `type: string`.
- Compare decoded input exactly, before escaping or HTTP execution. Preserve case, spaces, empty strings, Unicode, and URL-sensitive characters.
- Optional query omission is valid; explicit null is invalid. Empty input is valid only when listed.
- Resolve internal references and select the complete effective parameter after inheritance/override. An override may replace or remove the inherited enum.
- Publish enum members in the closed MCP input schema without changing requiredness.
- Reject numeric/boolean, mixed/null, duplicate or empty enums, and all previously unsupported additional schema constraints.

## Verification and continuation

RED was observed with missing enum metadata, then with missing MCP enum properties, unrestricted binding, and malformed metadata acceptance. Selector and CLI tests cover references, inheritance, replacement/removal, exact members, and rejection boundaries. MCP tests cover schema publication, direct binding, pre-network rejection, optional omission, and escaping. Container acceptance exercises inherited path plus overridden optional query over real stdio/HTTPS, invalid arguments, raw query preservation, and Bearer forwarding.

Local `go test -count=1 ./...`, `go vet ./...`, module tidy consistency, and fixture inspection passed. The container package compiled locally; execution failed the required prebuilt-image gate because this host has no Docker runtime. Actual Docker/stdio acceptance passed in [CI run 37237902608](https://github.com/kefyusuf/oasrelay/actions/runs/37237902608) on implementation commit `223980e63251a7b255569e9c4a7cf294abad0b6a`, including the new enum test and existing acceptance cases. The Linux verify job also passed. Independent source review found no actionable regression.

[PR #22](https://github.com/kefyusuf/oasrelay/pull/22) contains the test-contract commit `4181baf` followed by the implementation. This documentation checkpoint records the implementation-head evidence; GitHub Actions checks the final PR head separately. No merge or release is authorized by this increment. Keep continuation work bounded until maintainer review and merge; do not start a further schema feature in this PR.

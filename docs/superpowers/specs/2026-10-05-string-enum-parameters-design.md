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

Local Go tests and vet passed. The container package compiles locally; execution requires a prebuilt image and Docker, unavailable on this host. GitHub Actions must provide the actual Docker execution result before this PR is ready for review. No merge or release is authorized by this increment. Update this checkpoint with final PR/CI evidence after verification.

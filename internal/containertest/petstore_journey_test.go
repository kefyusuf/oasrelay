//go:build container

package containertest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDockerPetstoreInspectToServe checks the documented inspect -> select ->
// one-tool stdio call path against a pinned, published OpenAPI document.
// The upstream is a local TLS fixture; CI does not call the public Petstore API.
func TestDockerPetstoreInspectToServe(t *testing.T) {
	assertDockerToolCalls(t, "getUserByName", writePetstoreSpec, [2]containerToolCall{
		{"ordinary username", map[string]any{"username": "user1"}, "GET /api/v3/user/user1"},
		{"slash is one path segment", map[string]any{"username": "a/b"}, "GET /api/v3/user/a%2Fb"},
	},
		map[string]any{},
		map[string]any{"username": nil},
		map[string]any{"other": "unknown"},
	)
}

// writePetstoreSpec preserves the pinned Petstore operations and rewrites only
// the server URL to a local TLS fixture for offline container acceptance.
func writePetstoreSpec(t *testing.T, port int) string {
	t.Helper()

	document, err := os.ReadFile("../../testdata/petstore-openapi.yaml")
	if err != nil {
		t.Fatalf("read published Petstore specimen: %v", err)
	}
	const server = "  - url: https://petstore3.swagger.io/api/v3"
	if count := strings.Count(string(document), server); count != 1 {
		t.Fatalf("upstream Petstore server declaration count = %d, want 1", count)
	}
	// Change only the server origin, keeping the actual published operations,
	// parameters, schemas, and their unsupported forms unchanged.
	localSpec := strings.Replace(string(document), server,
		fmt.Sprintf("  - url: https://host.docker.internal:%d/api/v3", port), 1)
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte(localSpec), 0o644); err != nil {
		t.Fatalf("write local Petstore specimen: %v", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve local Petstore specimen: %v", err)
	}

	// No network is available during inspect: selection is purely static.
	image := strings.TrimSpace(os.Getenv("OASRELAY_IMAGE"))
	command := exec.Command("docker", "run", "--rm", "--network", "none",
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/work/openapi.yaml,readonly", absolute),
		image, "inspect", "--diagnostics", "/work/openapi.yaml")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect published Petstore specimen: %v; output = %q", err, output)
	}
	report := string(output)
	for _, expected := range []string{
		"getUserByName\n    GET /user/{username}\n    Selectable: yes",
		"getPetById\n    GET /pet/{petId}\n    Selectable: no",
		"findPetsByTags\n    GET /pet/findByTags\n    Selectable: no",
		"Diagnostics cover static selection only",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("Petstore diagnostics missing %q; report = %q", expected, report)
		}
	}
	return absolute
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func runInspectionWithoutServe(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	calls := 0
	code := runWithServe(args, &stdout, &stderr, func(context.Context, oasopenapi.SelectedOperation) error {
		calls++
		return nil
	})
	if calls != 0 {
		t.Fatalf("inspect invoked serve callback %d times", calls)
	}
	return code, stdout.String(), stderr.String()
}

func TestRunInspectDiagnosticsAcceptsSupportedEnumAndInheritedFixtures(t *testing.T) {
	for _, name := range []string{"string-enum-parameters.yaml", "inherited-parameters.yaml"} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runInspectionWithoutServe(t, "inspect", "--diagnostics", cliFixturePath(name))
			if code != 0 || stderr != "" || strings.Count(stdout, "Selectable: yes") != 1 || strings.Contains(stdout, "Selectable: no") {
				t.Fatalf("code = %d; stdout = %q; stderr = %q", code, stdout, stderr)
			}
			lower := strings.ToLower(stdout)
			for _, term := range []string{"static selection", "upstream", "authentication"} {
				if !strings.Contains(lower, term) {
					t.Fatalf("diagnostics report lacks disclaimer term %q: %q", term, stdout)
				}
			}
		})
	}
}

func TestRunInspectDiagnosticsReportsEachGETAndFirstSelectionReasonWithoutRequests(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "mixed.yaml")
	document := fmt.Sprintf(`openapi: 3.0.3
info:
  title: Mixed Diagnostics API
  version: 1.0.0
servers:
  - url: %s
paths:
  /accepted:
    get:
      operationId: accepted
      responses:
        '200':
          description: Accepted
  /rejected:
    get:
      operationId: rejected
      parameters:
        - name: first
          in: header
          schema:
            type: string
        - name: second
          in: query
          schema:
            type: string
        - name: third
          in: query
          schema:
            type: string
      requestBody:
        content:
          application/json:
            schema:
              type: string
      responses:
        '200':
          description: Rejected
  /ignored:
    post:
      operationId: ignored
      responses:
        '200':
          description: Ignored
`, upstream.URL)
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	code, stdout, stderr := runInspectionWithoutServe(t, "inspect", "--diagnostics", path)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d; stderr = %q", code, stderr)
	}
	accepted := strings.Index(stdout, "GET /accepted")
	rejected := strings.Index(stdout, "GET /rejected")
	if accepted < 0 || rejected <= accepted {
		t.Fatalf("missing or unordered operation blocks: %q", stdout)
	}
	if !strings.Contains(stdout[accepted:rejected], "Selectable: yes") || !strings.Contains(stdout[rejected:], "Selectable: no") ||
		!strings.Contains(stdout[rejected:], "at most two effective parameters") {
		t.Fatalf("missing per-operation selection diagnostics: %q", stdout)
	}
	if strings.Contains(stdout, "GET /ignored") || strings.Contains(stdout, "does not support request bodies") || strings.Contains(stdout, "query and path parameters only") {
		t.Fatalf("report includes non-GET operation or secondary rejection reasons: %q", stdout)
	}
	if requests.Load() != 0 {
		t.Fatalf("upstream requests = %d, want 0", requests.Load())
	}
}

func TestRunInspectDiagnosticsReportsMissingOperationID(t *testing.T) {
	code, stdout, stderr := runInspectionWithoutServe(t, "inspect", "--diagnostics", cliFixturePath("missing-operation-id.yaml"))
	if code != 0 || stderr != "" || !strings.Contains(stdout, "<missing operationId>") || !strings.Contains(stdout, "Selectable: no") {
		t.Fatalf("code = %d; stdout = %q; stderr = %q", code, stdout, stderr)
	}
}

func TestRunInspectDiagnosticsFalsePreservesLegacyOutput(t *testing.T) {
	for _, name := range []string{"customer-api.yaml", "missing-operation-id.yaml", "string-enum-parameters.yaml"} {
		t.Run(name, func(t *testing.T) {
			path := cliFixturePath(name)
			code, stdout, stderr := runInspectionWithoutServe(t, "inspect", path)
			falseCode, falseStdout, falseStderr := runInspectionWithoutServe(t, "inspect", "--diagnostics=false", path)
			if code != 0 || falseCode != code || falseStdout != stdout || falseStderr != stderr {
				t.Fatalf("legacy = (%d, %q, %q); diagnostics=false = (%d, %q, %q)", code, stdout, stderr, falseCode, falseStdout, falseStderr)
			}
			if strings.Contains(stdout, "Selectable:") || strings.Contains(strings.ToLower(stdout), "static selection") {
				t.Fatalf("legacy output includes diagnostic additions: %q", stdout)
			}
		})
	}
}

func TestRunInspectDiagnosticsKeepsDocumentAndUsageExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"invalid document", []string{"inspect", "--diagnostics", cliFixturePath("invalid-openapi.yaml")}, 1},
		{"missing path", []string{"inspect", "--diagnostics"}, 2},
		{"extra path", []string{"inspect", "--diagnostics", "one.yaml", "two.yaml"}, 2},
		{"unknown flag", []string{"inspect", "--unknown", cliFixturePath("single-get-api.yaml")}, 2},
		{"invalid boolean", []string{"inspect", "--diagnostics=maybe", cliFixturePath("single-get-api.yaml")}, 2},
		{"flag after path", []string{"inspect", cliFixturePath("single-get-api.yaml"), "--diagnostics"}, 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runInspectionWithoutServe(t, test.args...)
			if code != test.code || stdout != "" || stderr == "" {
				t.Fatalf("code = %d, want %d; stdout = %q; stderr = %q", code, test.code, stdout, stderr)
			}
		})
	}
}

func TestRunInspectDiagnosticsIgnoresMalformedBearerEnvironment(t *testing.T) {
	path := cliFixturePath("string-enum-parameters.yaml")
	t.Setenv("OASRELAY_BEARER_TOKEN", "")
	code, stdout, stderr := runInspectionWithoutServe(t, "inspect", "--diagnostics", path)
	const secret = "diagnostics-secret\r\nmalformed"
	t.Setenv("OASRELAY_BEARER_TOKEN", secret)
	secretCode, secretStdout, secretStderr := runInspectionWithoutServe(t, "inspect", "--diagnostics", path)
	if code != 0 || secretCode != code || secretStdout != stdout || secretStderr != stderr {
		t.Fatal("bearer environment changed static diagnostics")
	}
	if strings.Contains(secretStdout+secretStderr, "diagnostics-secret") || strings.Contains(secretStdout+secretStderr, "malformed") {
		t.Fatal("bearer secret leaked into diagnostics output")
	}
}

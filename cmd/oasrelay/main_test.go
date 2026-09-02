package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func cliFixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

func TestRunInspectPrintsDeterministicReport(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"inspect", cliFixturePath("customer-api.yaml")}, &stdout, &stderr)

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code = %d; stderr = %q", code, stderr.String())
	}

	want := `OpenAPI: 3.0.3
API: Customer API
Version: 1.0.0
Server: http://localhost:8080

Available GET operations:

  listCustomers
    GET /customers

  getCustomer
    GET /customers/{customerId}

Total GET operations: 2
`
	if stdout.String() != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s", stdout.String(), want)
	}
}

func TestRunInspectWarnsAboutMissingOperationID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"inspect", cliFixturePath("missing-operation-id.yaml")}, &stdout, &stderr)

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code = %d; stderr = %q", code, stderr.String())
	}
	for _, expected := range []string{
		"<missing operationId>",
		"GET /health",
		"Warning: operationId is required for future MCP exposure",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), expected)
		}
	}
	if strings.Contains(stdout.String(), "Server:") {
		t.Fatalf("stdout = %q, did not want a server line", stdout.String())
	}
}

func TestRunServeSelectsOperationAndDelegates(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var selected oasopenapi.SelectedOperation
	calls := 0

	code := runWithServe(
		[]string{"serve", "--operation-id", "listCustomers", cliFixturePath("single-get-api.yaml")},
		&stdout,
		&stderr,
		func(_ context.Context, operation oasopenapi.SelectedOperation) error {
			calls++
			selected = operation
			return nil
		},
	)

	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 || calls != 1 {
		t.Fatalf(
			"code = %d; stdout = %q; stderr = %q; calls = %d",
			code,
			stdout.String(),
			stderr.String(),
			calls,
		)
	}
	if selected.OperationID != "listCustomers" ||
		selected.Endpoint != "https://document.example.test/api/customers" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestRunServeRejectsInvalidUsage(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		detail string
	}{
		{
			name:   "missing operation ID",
			args:   []string{"serve", cliFixturePath("single-get-api.yaml")},
			detail: "--operation-id is required",
		},
		{
			name:   "missing spec path",
			args:   []string{"serve", "--operation-id", "listCustomers"},
			detail: "serve requires exactly one local spec path",
		},
		{
			name: "excess spec paths",
			args: []string{
				"serve",
				"--operation-id",
				"listCustomers",
				"one.yaml",
				"two.yaml",
			},
			detail: "serve requires exactly one local spec path",
		},
		{
			name:   "unknown flag",
			args:   []string{"serve", "--unknown", "value", cliFixturePath("single-get-api.yaml")},
			detail: "invalid serve arguments",
		},
		{
			name: "flag after positional path",
			args: []string{
				"serve",
				cliFixturePath("single-get-api.yaml"),
				"--operation-id",
				"listCustomers",
			},
			detail: "serve requires exactly one local spec path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			calls := 0

			code := runWithServe(
				test.args,
				&stdout,
				&stderr,
				func(context.Context, oasopenapi.SelectedOperation) error {
					calls++
					return nil
				},
			)

			if code != 2 || stdout.Len() != 0 || calls != 0 ||
				!strings.Contains(stderr.String(), test.detail) ||
				!strings.Contains(
					stderr.String(),
					"usage: oasrelay serve --operation-id <id> <local-spec-path>",
				) {
				t.Fatalf(
					"code = %d; stdout = %q; stderr = %q; calls = %d",
					code,
					stdout.String(),
					stderr.String(),
					calls,
				)
			}
		})
	}
}

func TestRunServeReturnsSelectionErrorBeforeStartingMCP(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	calls := 0

	code := runWithServe(
		[]string{"serve", "--operation-id", "createCustomer", cliFixturePath("single-get-api.yaml")},
		&stdout,
		&stderr,
		func(context.Context, oasopenapi.SelectedOperation) error {
			calls++
			return nil
		},
	)

	if code != 1 || stdout.Len() != 0 || calls != 0 ||
		!strings.Contains(stderr.String(), "uses POST; this version supports GET only") {
		t.Fatalf(
			"code = %d; stdout = %q; stderr = %q; calls = %d",
			code,
			stdout.String(),
			stderr.String(),
			calls,
		)
	}
}

func TestRunServeReturnsRuntimeError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithServe(
		[]string{"serve", "--operation-id", "listCustomers", cliFixturePath("single-get-api.yaml")},
		&stdout,
		&stderr,
		func(context.Context, oasopenapi.SelectedOperation) error {
			return errors.New("stdio failed")
		},
	)

	if code != 1 || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "serve MCP server: stdio failed") {
		t.Fatalf("code = %d; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
}

func TestRunReturnsUsageErrorForMissingCommand(t *testing.T) {
	assertUsageError(t, nil, "")
}

func TestRunReturnsUsageErrorForMissingInspectPath(t *testing.T) {
	assertUsageError(t, []string{"inspect"}, "inspect requires exactly one local spec path")
}

func TestRunReturnsUsageErrorForExcessInspectArguments(t *testing.T) {
	assertUsageError(t, []string{"inspect", "one.yaml", "two.yaml"}, "inspect requires exactly one local spec path")
}

func TestRunReturnsUsageErrorForUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"unknown"}, &stdout, &stderr)

	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `unknown command "unknown"`) ||
		!strings.Contains(stderr.String(), "usage: oasrelay inspect <local-spec-path>") ||
		!strings.Contains(stderr.String(), "usage: oasrelay serve --operation-id <id> <local-spec-path>") {
		t.Fatalf("code = %d; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
}

func TestRunReturnsOperationalErrorForInvalidDocument(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	path := cliFixturePath("invalid-openapi.yaml")
	code := run([]string{"inspect", path}, &stdout, &stderr)

	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), path) ||
		!strings.Contains(stderr.String(), "validate document") {
		t.Fatalf("code = %d; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
}

func assertUsageError(t *testing.T, args []string, detail string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, &stdout, &stderr)

	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage: oasrelay inspect <local-spec-path>") {
		t.Fatalf("code = %d; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
	if detail != "" && !strings.Contains(stderr.String(), detail) {
		t.Fatalf("stderr = %q, want %q", stderr.String(), detail)
	}
}

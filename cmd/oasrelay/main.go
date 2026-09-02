package main

import (
	"fmt"
	"io"
	"os"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

const usage = "usage: oasrelay inspect <local-spec-path>"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if args[0] != "inspect" {
		fmt.Fprintf(stderr, "error: unknown command %q\n", args[0])
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if len(args) != 2 {
		fmt.Fprintln(stderr, "error: inspect requires exactly one local spec path")
		fmt.Fprintln(stderr, usage)
		return 2
	}

	inspection, err := oasopenapi.InspectFile(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "error: inspect %s: %v\n", args[1], err)
		return 1
	}

	writeInspection(stdout, inspection)
	return 0
}

func writeInspection(w io.Writer, inspection oasopenapi.Inspection) {
	fmt.Fprintf(w, "OpenAPI: %s\n", inspection.OpenAPIVersion)
	fmt.Fprintf(w, "API: %s\n", inspection.Title)
	fmt.Fprintf(w, "Version: %s\n", inspection.APIVersion)
	if inspection.ServerURL != "" {
		fmt.Fprintf(w, "Server: %s\n", inspection.ServerURL)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Available GET operations:")
	if len(inspection.Operations) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  (none)")
	}

	for _, operation := range inspection.Operations {
		operationID := operation.OperationID
		if operationID == "" {
			operationID = "<missing operationId>"
		}

		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", operationID)
		fmt.Fprintf(w, "    %s %s\n", operation.Method, operation.Path)
		if operation.OperationID == "" {
			fmt.Fprintln(w, "    Warning: operationId is required for future MCP exposure")
		}
	}

	fmt.Fprintf(w, "\nTotal GET operations: %d\n", len(inspection.Operations))
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kefyusuf/oasrelay/internal/mcpserver"
	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

const (
	inspectUsage = "usage: oasrelay inspect <local-spec-path>"
	serveUsage   = "usage: oasrelay serve --operation-id <id> <local-spec-path>"
)

type serveFunc func(context.Context, oasopenapi.SelectedOperation) error

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithServe(args, stdout, stderr, mcpserver.RunStdio)
}

func runWithServe(args []string, stdout, stderr io.Writer, serve serveFunc) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}

	switch args[0] {
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stderr, serve)
	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n", args[0])
		writeUsage(stderr)
		return 2
	}
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "error: inspect requires exactly one local spec path")
		fmt.Fprintln(stderr, inspectUsage)
		return 2
	}

	inspection, err := oasopenapi.InspectFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "error: inspect %s: %v\n", args[0], err)
		return 1
	}

	writeInspection(stdout, inspection)
	return 0
}

func runServe(args []string, stderr io.Writer, serve serveFunc) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var operationID string
	flags.StringVar(&operationID, "operation-id", "", "OpenAPI operationId to expose")

	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "error: invalid serve arguments: %v\n", err)
		fmt.Fprintln(stderr, serveUsage)
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "error: serve requires exactly one local spec path")
		fmt.Fprintln(stderr, serveUsage)
		return 2
	}
	if operationID == "" {
		fmt.Fprintln(stderr, "error: --operation-id is required")
		fmt.Fprintln(stderr, serveUsage)
		return 2
	}

	operation, err := oasopenapi.SelectGET(flags.Arg(0), operationID)
	if err != nil {
		fmt.Fprintf(stderr, "error: select operation: %v\n", err)
		return 1
	}
	if err := serve(context.Background(), operation); err != nil {
		fmt.Fprintf(stderr, "error: serve MCP server: %v\n", err)
		return 1
	}

	return 0
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, inspectUsage)
	fmt.Fprintln(w, serveUsage)
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

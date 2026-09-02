package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/kefyusuf/oasrelay/internal/upstream"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	implementationName    = "oasrelay"
	implementationVersion = "0.0.0-dev"
)

// ToolInput is the intentionally empty argument object for the first supported
// parameterless OpenAPI operation shape.
type ToolInput struct{}

// ToolOutput is the bounded raw HTTP response returned by the MCP tool.
type ToolOutput struct {
	Status      int    `json:"status" jsonschema:"HTTP response status code"`
	ContentType string `json:"contentType" jsonschema:"HTTP Content-Type response header"`
	Body        string `json:"body" jsonschema:"raw HTTP response body"`
}

// New creates an MCP server that exposes exactly one upstream-backed tool.
func New(operation oasopenapi.SelectedOperation, client *http.Client) (*mcp.Server, error) {
	if client == nil {
		return nil, fmt.Errorf("HTTP client is required")
	}
	if err := validateMCPToolName(operation.OperationID); err != nil {
		return nil, fmt.Errorf(
			"operationId %q is not a valid MCP tool name: %w",
			operation.OperationID,
			err,
		)
	}

	server := mcp.NewServer(
		&mcp.Implementation{Name: implementationName, Version: implementationVersion},
		nil,
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        operation.OperationID,
			Description: toolDescription(operation),
		},
		func(
			ctx context.Context,
			_ *mcp.CallToolRequest,
			_ ToolInput,
		) (*mcp.CallToolResult, ToolOutput, error) {
			response, err := upstream.Get(ctx, client, operation.Endpoint)
			if err != nil {
				return nil, ToolOutput{}, err
			}

			output := ToolOutput{
				Status:      response.Status,
				ContentType: response.ContentType,
				Body:        response.Body,
			}
			if response.Status < http.StatusOK || response.Status >= http.StatusMultipleChoices {
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, output, nil
		},
	)

	return server, nil
}

// RunStdio runs the one-tool server on MCP's standard input/output transport.
func RunStdio(ctx context.Context, operation oasopenapi.SelectedOperation) error {
	client := &http.Client{Timeout: upstream.RequestTimeout}
	server, err := New(operation, client)
	if err != nil {
		return err
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

func toolDescription(operation oasopenapi.SelectedOperation) string {
	summary := strings.TrimSpace(operation.Summary)
	description := strings.TrimSpace(operation.Description)

	switch {
	case summary != "" && description != "":
		return summary + "\n\n" + description
	case summary != "":
		return summary
	case description != "":
		return description
	default:
		return fmt.Sprintf("%s %s", operation.Method, operation.Path)
	}
}

func validateMCPToolName(name string) error {
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("tool name exceeds 128 characters")
	}
	for _, character := range name {
		if isMCPToolNameCharacter(character) {
			continue
		}
		return fmt.Errorf("tool name contains invalid character %q", character)
	}
	return nil
}

func isMCPToolNameCharacter(character rune) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' ||
		character == '_' || character == '-' || character == '.'
}

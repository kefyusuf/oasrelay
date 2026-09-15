package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
	"github.com/kefyusuf/oasrelay/internal/upstream"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	implementationName    = "oasrelay"
	implementationVersion = "0.0.0-dev"
)

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
			InputSchema: toolInputSchema(operation),
		},
		func(
			ctx context.Context,
			_ *mcp.CallToolRequest,
			input map[string]json.RawMessage,
		) (*mcp.CallToolResult, ToolOutput, error) {
			endpoint, err := bindQueryParameter(operation.Endpoint, operation.QueryParameter, input)
			if err != nil {
				return nil, ToolOutput{}, err
			}

			response, err := upstream.Get(ctx, client, endpoint)
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

func toolInputSchema(operation oasopenapi.SelectedOperation) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
	if operation.QueryParameter == nil {
		return schema
	}

	parameter := operation.QueryParameter
	schema["properties"] = map[string]any{
		parameter.Name: map[string]any{"type": parameter.Type},
	}
	schema["required"] = []string{parameter.Name}
	return schema
}

func bindQueryParameter(
	endpoint string,
	parameter *oasopenapi.QueryParameter,
	input map[string]json.RawMessage,
) (string, error) {
	if parameter == nil {
		if len(input) != 0 {
			return "", fmt.Errorf("parameterless tool does not accept arguments")
		}
		return endpoint, nil
	}

	if len(input) != 1 {
		return "", fmt.Errorf("tool requires exactly query parameter %q", parameter.Name)
	}
	raw, ok := input[parameter.Name]
	if !ok {
		return "", fmt.Errorf("required query parameter %q is missing", parameter.Name)
	}

	value, err := primitiveQueryValue(parameter.Type, raw)
	if err != nil {
		return "", fmt.Errorf("query parameter %q: %w", parameter.Name, err)
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}
	query := parsed.Query()
	query.Set(parameter.Name, value)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func primitiveQueryValue(parameterType string, raw json.RawMessage) (string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", fmt.Errorf("expected non-null %s", parameterType)
	}

	switch parameterType {
	case "string":
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", fmt.Errorf("expected string: %w", err)
		}
		return value, nil
	case "boolean":
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", fmt.Errorf("expected boolean: %w", err)
		}
		if value {
			return "true", nil
		}
		return "false", nil
	case "integer":
		var value json.Number
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", fmt.Errorf("expected integer: %w", err)
		}
		rational, ok := new(big.Rat).SetString(value.String())
		if !ok || !rational.IsInt() {
			return "", fmt.Errorf("expected integer")
		}
		return rational.Num().String(), nil
	case "number":
		var value json.Number
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", fmt.Errorf("expected number: %w", err)
		}
		return value.String(), nil
	default:
		return "", fmt.Errorf("unsupported primitive type %q", parameterType)
	}
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

package openapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// SelectedOperation is the validated project-owned operation required by the
// one-tool MCP runtime.
type SelectedOperation struct {
	OperationID string
	Method      string
	Path        string
	Summary     string
	Description string
	Endpoint    string
}

// SelectParameterlessGET loads one local document and selects the exact
// operationId when it represents the deliberately restricted GET shape
// supported by the first MCP runtime slice.
func SelectParameterlessGET(path, operationID string) (SelectedOperation, error) {
	document, err := loadDocument(path)
	if err != nil {
		return SelectedOperation{}, err
	}

	if document.Paths != nil {
		for route, item := range document.Paths.Map() {
			if item == nil {
				continue
			}
			for method, operation := range item.Operations() {
				if operation == nil || operation.OperationID != operationID {
					continue
				}
				return selectOperation(document, route, method, item, operation)
			}
		}
	}

	return SelectedOperation{}, fmt.Errorf("operationId %q was not found", operationID)
}

func selectOperation(
	document *openapi3.T,
	route, method string,
	item *openapi3.PathItem,
	operation *openapi3.Operation,
) (SelectedOperation, error) {
	operationID := operation.OperationID

	if method != http.MethodGet {
		return SelectedOperation{}, fmt.Errorf(
			"operationId %q uses %s; this version supports GET only",
			operationID,
			method,
		)
	}
	if len(item.Parameters) != 0 || len(operation.Parameters) != 0 {
		return SelectedOperation{}, fmt.Errorf(
			"operationId %q has parameters; this version supports parameterless operations only",
			operationID,
		)
	}
	if operation.RequestBody != nil {
		return SelectedOperation{}, fmt.Errorf(
			"operationId %q has a request body; this version does not support request bodies",
			operationID,
		)
	}
	if err := validateMCPToolName(operationID); err != nil {
		return SelectedOperation{}, fmt.Errorf(
			"operationId %q is not a valid MCP tool name: %w",
			operationID,
			err,
		)
	}

	endpoint, err := endpointFor(effectiveServer(document, item, operation), route)
	if err != nil {
		return SelectedOperation{}, fmt.Errorf("operationId %q: %w", operationID, err)
	}

	return SelectedOperation{
		OperationID: operationID,
		Method:      http.MethodGet,
		Path:        route,
		Summary:     operation.Summary,
		Description: operation.Description,
		Endpoint:    endpoint,
	}, nil
}

func effectiveServer(
	document *openapi3.T,
	item *openapi3.PathItem,
	operation *openapi3.Operation,
) *openapi3.Server {
	if len(operation.Servers) > 0 {
		return operation.Servers[0]
	}
	if len(item.Servers) > 0 {
		return item.Servers[0]
	}
	if len(document.Servers) > 0 {
		return document.Servers[0]
	}
	return nil
}

func endpointFor(server *openapi3.Server, route string) (string, error) {
	if server == nil || server.URL == "" {
		return "", fmt.Errorf("has no usable absolute HTTP(S) server URL")
	}
	if len(server.Variables) != 0 || strings.ContainsAny(server.URL, "{}") {
		return "", fmt.Errorf("uses server variables; this version supports static server URLs only")
	}

	parsed, err := url.Parse(server.URL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("has no usable absolute HTTP(S) server URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("has no usable absolute HTTP(S) server URL")
	}

	endpoint, err := url.JoinPath(parsed.String(), strings.TrimPrefix(route, "/"))
	if err != nil {
		return "", fmt.Errorf("build endpoint: %w", err)
	}
	return endpoint, nil
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

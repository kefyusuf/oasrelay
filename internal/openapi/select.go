package openapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// QueryParameter is the deliberately small query-parameter contract supported
// by the one-tool MCP runtime.
type QueryParameter struct {
	Name     string
	Type     string
	Optional bool
}

// PathParameter is the deliberately small path-parameter contract supported
// by the one-tool MCP runtime.
type PathParameter struct {
	Name string
	Type string
}

// SelectedOperation is the validated project-owned operation required by the
// one-tool MCP runtime.
type SelectedOperation struct {
	OperationID     string
	Method          string
	Path            string
	Summary         string
	Description     string
	Endpoint        string
	QueryParameters []QueryParameter
	PathParameters  []PathParameter
}

// SelectParameterlessGET loads one local document and selects the exact
// operationId when it represents the deliberately restricted parameterless GET
// shape supported by the original MCP runtime slice.
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

// SelectGET loads one local document and selects the exact operationId when it
// represents a GET with up to two supported effective parameters after path-item
// inheritance and operation overrides: query, required path, or one of each.
func SelectGET(path, operationID string) (SelectedOperation, error) {
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
				return selectGETOperation(document, route, method, item, operation)
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

	return buildSelectedOperation(document, route, method, item, operation, nil, nil)
}

func selectGETOperation(
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
	parameters, err := effectiveParameters(operationID, item.Parameters, operation.Parameters)
	if err != nil {
		return SelectedOperation{}, err
	}
	if len(parameters) > 2 {
		return SelectedOperation{}, fmt.Errorf(
			"operationId %q has %d effective parameters; this version supports at most two effective parameters",
			operationID,
			len(parameters),
		)
	}
	if operation.RequestBody != nil {
		return SelectedOperation{}, fmt.Errorf(
			"operationId %q has a request body; this version does not support request bodies",
			operationID,
		)
	}

	queryParameters, pathParameters, err := supportedOperationParameters(operationID, route, parameters)
	if err != nil {
		return SelectedOperation{}, err
	}
	return buildSelectedOperation(document, route, method, item, operation, queryParameters, pathParameters)
}

// effectiveParameters validates each declaration source before merging by the
// OpenAPI (name, in) identity. Overrides retain the inherited declaration slot.
func effectiveParameters(operationID string, inherited, declared openapi3.Parameters) (openapi3.Parameters, error) {
	type identity struct{ name, location string }
	parameters := make(openapi3.Parameters, 0, len(inherited)+len(declared))
	slots := make(map[identity]int, len(inherited)+len(declared))
	for _, source := range []struct {
		name       string
		parameters openapi3.Parameters
	}{{"path-item", inherited}, {"operation", declared}} {
		seen := make(map[identity]bool, len(source.parameters))
		for _, ref := range source.parameters {
			if ref == nil || ref.Value == nil {
				return nil, fmt.Errorf("operationId %q has an unresolved %s parameter; this version requires resolved parameters", operationID, source.name)
			}
			key := identity{ref.Value.Name, ref.Value.In}
			if seen[key] {
				return nil, fmt.Errorf("operationId %q has duplicate %s parameter name %q in %s", operationID, source.name, key.name, key.location)
			}
			seen[key] = true
			if slot, exists := slots[key]; exists {
				parameters[slot] = ref
			} else {
				slots[key] = len(parameters)
				parameters = append(parameters, ref)
			}
		}
	}
	return parameters, nil
}

func supportedOperationParameters(
	operationID, route string,
	parameters openapi3.Parameters,
) ([]QueryParameter, []PathParameter, error) {
	queryRaw := make([]*openapi3.Parameter, 0, 2)
	pathRaw := make([]*openapi3.Parameter, 0, 2)

	for _, parameterRef := range parameters {
		if parameterRef == nil || parameterRef.Value == nil {
			return nil, nil, fmt.Errorf(
				"operationId %q has an unresolved parameter; this version requires resolved operation parameters",
				operationID,
			)
		}

		parameter := parameterRef.Value
		switch parameter.In {
		case openapi3.ParameterInQuery:
			if len(queryRaw) == 2 {
				return nil, nil, fmt.Errorf(
					"operationId %q supports at most two query parameters",
					operationID,
				)
			}
			for _, existing := range queryRaw {
				if existing.Name == parameter.Name {
					return nil, nil, fmt.Errorf(
						"operationId %q has duplicate query parameter name %q",
						operationID,
						parameter.Name,
					)
				}
			}
			queryRaw = append(queryRaw, parameter)

		case openapi3.ParameterInPath:
			if len(pathRaw) == 2 {
				return nil, nil, fmt.Errorf(
					"operationId %q supports at most two path parameters",
					operationID,
				)
			}
			for _, existing := range pathRaw {
				if existing.Name == parameter.Name {
					return nil, nil, fmt.Errorf("operationId %q has duplicate path parameter name %q", operationID, parameter.Name)
				}
			}
			pathRaw = append(pathRaw, parameter)

		default:
			return nil, nil, fmt.Errorf(
				"operationId %q parameter %q is in %s; this version supports query and path parameters only",
				operationID,
				parameter.Name,
				parameter.In,
			)
		}
	}

	if len(pathRaw)+len(queryRaw) > 2 {
		return nil, nil, fmt.Errorf("operationId %q supports at most two effective parameters", operationID)
	}
	for _, path := range pathRaw {
		for _, query := range queryRaw {
			if query.Name == path.Name {
				return nil, nil, fmt.Errorf(
					"operationId %q path and query parameters share parameter name %q; MCP input names must be unique",
					operationID,
					query.Name,
				)
			}
		}
	}

	queryParameters := make([]QueryParameter, 0, len(queryRaw))
	for _, raw := range queryRaw {
		parameter, err := supportedQueryParameter(operationID, raw)
		if err != nil {
			return nil, nil, err
		}
		queryParameters = append(queryParameters, *parameter)
	}

	pathParameters := make([]PathParameter, 0, len(pathRaw))
	remainingRoute := route
	for _, raw := range pathRaw {
		parameter, err := supportedPathParameter(operationID, route, raw)
		if err != nil {
			return nil, nil, err
		}
		pathParameters = append(pathParameters, *parameter)
		remainingRoute = strings.Replace(remainingRoute, "{"+parameter.Name+"}", "", 1)
	}
	if strings.ContainsAny(remainingRoute, "{}") {
		return nil, nil, fmt.Errorf("operationId %q route contains additional path placeholders", operationID)
	}

	return queryParameters, pathParameters, nil
}

func supportedQueryParameter(
	operationID string,
	parameter *openapi3.Parameter,
) (*QueryParameter, error) {
	if parameter == nil {
		return nil, fmt.Errorf(
			"operationId %q has an unresolved query parameter",
			operationID,
		)
	}
	if parameter.In != openapi3.ParameterInQuery {
		return nil, fmt.Errorf(
			"operationId %q parameter %q is in %s; this version supports query parameters only",
			operationID,
			parameter.Name,
			parameter.In,
		)
	}
	serialization, err := parameter.SerializationMethod()
	if err != nil {
		return nil, fmt.Errorf("operationId %q query parameter %q: %w", operationID, parameter.Name, err)
	}
	if serialization.Style != "form" || !serialization.Explode || parameter.AllowReserved {
		return nil, fmt.Errorf(
			"operationId %q query parameter %q does not use default query serialization",
			operationID,
			parameter.Name,
		)
	}

	if parameter.Schema == nil || parameter.Schema.Value == nil || !isPlainPrimitiveSchema(parameter.Schema.Value) {
		return nil, fmt.Errorf(
			"operationId %q query parameter %q must use a plain primitive schema",
			operationID,
			parameter.Name,
		)
	}

	return &QueryParameter{
		Name:     parameter.Name,
		Type:     (*parameter.Schema.Value.Type)[0],
		Optional: !parameter.Required,
	}, nil
}

func supportedPathParameter(
	operationID, route string,
	parameter *openapi3.Parameter,
) (*PathParameter, error) {
	if !parameter.Required {
		return nil, fmt.Errorf(
			"operationId %q path parameter %q must be required",
			operationID,
			parameter.Name,
		)
	}

	serialization, err := parameter.SerializationMethod()
	if err != nil {
		return nil, fmt.Errorf("operationId %q path parameter %q: %w", operationID, parameter.Name, err)
	}
	if serialization.Style != "simple" || serialization.Explode {
		return nil, fmt.Errorf(
			"operationId %q path parameter %q does not use default path serialization",
			operationID,
			parameter.Name,
		)
	}

	if parameter.Schema == nil || parameter.Schema.Value == nil || !isPlainPrimitiveSchema(parameter.Schema.Value) {
		return nil, fmt.Errorf(
			"operationId %q path parameter %q must use a plain primitive schema",
			operationID,
			parameter.Name,
		)
	}

	placeholder := "{" + parameter.Name + "}"
	if strings.Count(route, placeholder) != 1 {
		return nil, fmt.Errorf(
			"operationId %q path parameter %q must match exactly one path placeholder",
			operationID,
			parameter.Name,
		)
	}

	return &PathParameter{
		Name: parameter.Name,
		Type: (*parameter.Schema.Value.Type)[0],
	}, nil
}

func isPlainPrimitiveSchema(schema *openapi3.Schema) bool {
	if schema == nil || schema.Type == nil || !schema.Type.IsSingle() {
		return false
	}

	typeName := (*schema.Type)[0]
	switch typeName {
	case openapi3.TypeString, openapi3.TypeInteger, openapi3.TypeNumber, openapi3.TypeBoolean:
	default:
		return false
	}

	return len(schema.OneOf) == 0 &&
		len(schema.AnyOf) == 0 &&
		len(schema.AllOf) == 0 &&
		schema.Not == nil &&
		!schema.Nullable &&
		len(schema.Enum) == 0 &&
		schema.Default == nil &&
		schema.Format == "" &&
		schema.Min == nil &&
		schema.Max == nil &&
		!hasExclusiveBound(schema.ExclusiveMin) &&
		!hasExclusiveBound(schema.ExclusiveMax) &&
		schema.MultipleOf == nil &&
		schema.MinLength == 0 &&
		schema.MaxLength == nil &&
		schema.Pattern == "" &&
		schema.Items == nil &&
		len(schema.Properties) == 0 &&
		schema.Const == nil &&
		schema.If == nil &&
		schema.Then == nil &&
		schema.Else == nil
}

func hasExclusiveBound(bound openapi3.ExclusiveBound) bool {
	return bound.Value != nil || bound.Bool != nil && *bound.Bool
}

func buildSelectedOperation(
	document *openapi3.T,
	route, method string,
	item *openapi3.PathItem,
	operation *openapi3.Operation,
	queryParameters []QueryParameter,
	pathParameters []PathParameter,
) (SelectedOperation, error) {
	operationID := operation.OperationID
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
		OperationID:     operationID,
		Method:          http.MethodGet,
		Path:            route,
		Summary:         operation.Summary,
		Description:     operation.Description,
		Endpoint:        endpoint,
		QueryParameters: queryParameters,
		PathParameters:  pathParameters,
	}, nil
}

func effectiveServer(
	document *openapi3.T,
	item *openapi3.PathItem,
	operation *openapi3.Operation,
) *openapi3.Server {
	if operation.Servers != nil && len(*operation.Servers) > 0 {
		return (*operation.Servers)[0]
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

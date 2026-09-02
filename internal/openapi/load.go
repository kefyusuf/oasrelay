package openapi

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

func loadDocument(path string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	document, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load document: %w", err)
	}
	if document.OpenAPIMajorMinor() == "" {
		return nil, fmt.Errorf(
			"unsupported OpenAPI version %q: expected a supported 3.x version",
			document.OpenAPI,
		)
	}
	if err := document.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("validate document: %w", err)
	}

	return document, nil
}

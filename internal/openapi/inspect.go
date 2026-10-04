package openapi

import "sort"

// Inspection is the small, project-owned view of an OpenAPI document that the
// CLI needs. It deliberately does not expose kin-openapi types.
type Inspection struct {
	OpenAPIVersion string
	Title          string
	APIVersion     string
	ServerURL      string
	Operations     []Operation
	Diagnostics    map[string]SelectionDiagnostic
}

// SelectionDiagnostic describes static selector acceptance, not runtime readiness.
type SelectionDiagnostic struct {
	Selectable bool
	Reason     string
}

// Operation describes one discovered GET operation.
type Operation struct {
	OperationID string
	Method      string
	Path        string
}

// InspectFile loads and validates one local OpenAPI document, then returns its
// metadata and GET operations. External references remain disabled.
func InspectFile(path string) (Inspection, error) {
	return inspectFile(path, false)
}

// InspectFileWithDiagnostics adds static GET selection diagnostics keyed by path.
// It loads the document once and never starts MCP or contacts an upstream.
func InspectFileWithDiagnostics(path string) (Inspection, error) {
	return inspectFile(path, true)
}

func inspectFile(path string, diagnostics bool) (Inspection, error) {
	document, err := loadDocument(path)
	if err != nil {
		return Inspection{}, err
	}

	result := Inspection{
		OpenAPIVersion: document.OpenAPI,
		Title:          document.Info.Title,
		APIVersion:     document.Info.Version,
	}
	if len(document.Servers) > 0 && document.Servers[0] != nil {
		result.ServerURL = document.Servers[0].URL
	}
	if diagnostics {
		result.Diagnostics = make(map[string]SelectionDiagnostic)
	}

	if document.Paths != nil {
		for path, item := range document.Paths.Map() {
			if item == nil || item.Get == nil {
				continue
			}
			result.Operations = append(result.Operations, Operation{
				OperationID: item.Get.OperationID,
				Method:      "GET",
				Path:        path,
			})
			if diagnostics {
				_, err := selectGETOperation(document, path, "GET", item, item.Get)
				diagnostic := SelectionDiagnostic{Selectable: err == nil}
				if err != nil {
					diagnostic.Reason = err.Error()
				}
				result.Diagnostics[path] = diagnostic
			}
		}
	}

	sort.Slice(result.Operations, func(i, j int) bool {
		if result.Operations[i].Path == result.Operations[j].Path {
			return result.Operations[i].OperationID < result.Operations[j].OperationID
		}
		return result.Operations[i].Path < result.Operations[j].Path
	})

	return result, nil
}

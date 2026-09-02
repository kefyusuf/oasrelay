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

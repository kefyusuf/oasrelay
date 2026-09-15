package mcpserver

import (
	"encoding/json"
	"testing"
)

func TestPrimitiveQueryValueRejectsNull(t *testing.T) {
	for _, parameterType := range []string{"string", "integer", "number", "boolean"} {
		t.Run(parameterType, func(t *testing.T) {
			if _, err := primitiveQueryValue(parameterType, json.RawMessage(`null`)); err == nil {
				t.Fatalf("primitiveQueryValue(%q, null) error = nil, want rejection", parameterType)
			}
		})
	}
}

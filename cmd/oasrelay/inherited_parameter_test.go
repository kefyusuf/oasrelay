package main

import (
	"bytes"
	"context"
	"testing"

	oasopenapi "github.com/kefyusuf/oasrelay/internal/openapi"
)

func TestRunServeAcceptsInheritedParametersWithOperationOverride(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var selected oasopenapi.SelectedOperation
	calls := 0
	code := runWithServe(
		[]string{"serve", "--operation-id", "getCustomerOrders", "../../testdata/inherited-parameters.yaml"},
		&stdout, &stderr,
		func(_ context.Context, operation oasopenapi.SelectedOperation) error {
			calls++
			selected = operation
			return nil
		},
	)
	if code != 0 || calls != 1 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, calls, stdout.String(), stderr.String())
	}
	if len(selected.PathParameters) != 1 || selected.PathParameters[0].Name != "customerId" {
		t.Fatalf("inherited path = %#v", selected.PathParameters)
	}
	if len(selected.QueryParameters) != 1 || selected.QueryParameters[0] != (oasopenapi.QueryParameter{Name: "limit", Type: "integer", Optional: true}) {
		t.Fatalf("overridden query = %#v", selected.QueryParameters)
	}
}

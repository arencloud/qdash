package validation

import "testing"

func TestValidateResourceSpecGatewayValid(t *testing.T) {
	spec := map[string]any{
		"gatewayClassName": "openshift-default",
		"listeners": []any{
			map[string]any{"name": "http", "protocol": "HTTP", "port": 80},
		},
	}
	if errs := ValidateResourceSpec("gateways", spec); len(errs) != 0 {
		t.Fatalf("expected no errors, got %+v", errs)
	}
}

func TestValidateResourceSpecGatewayInvalid(t *testing.T) {
	spec := map[string]any{
		"listeners": []any{
			map[string]any{"name": "dup", "protocol": "HTTP", "port": 0},
			map[string]any{"name": "dup", "protocol": "", "port": 70000},
		},
	}
	errs := ValidateResourceSpec("gateways", spec)
	if len(errs) < 4 {
		t.Fatalf("expected multiple errors, got %+v", errs)
	}
}

func TestValidateResourceSpecRateLimitInvalidWindow(t *testing.T) {
	spec := map[string]any{
		"targetRef": map[string]any{"kind": "HTTPRoute", "name": "route-a"},
		"limits": map[string]any{
			"tenant-a": map[string]any{
				"rates": []any{
					map[string]any{"limit": 10, "window": "bad-window"},
				},
			},
		},
	}
	errs := ValidateResourceSpec("ratelimitpolicies", spec)
	if len(errs) == 0 {
		t.Fatalf("expected validation error for invalid window")
	}
	found := false
	for _, err := range errs {
		if err.Field == "spec.limits.tenant-a.rates[0].window" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected window field error, got %+v", errs)
	}
}

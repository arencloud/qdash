package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

var rateWindowRe = regexp.MustCompile(`^([0-9]{1,5}(h|m|s|ms)){1,4}$`)

func ValidateResourceSpec(resource string, spec map[string]any) []FieldError {
	switch strings.ToLower(strings.TrimSpace(resource)) {
	case "gateways":
		return validateGateway(spec)
	case "httproutes":
		return validateHTTPRoute(spec)
	case "authpolicies":
		return validateAuthPolicy(spec)
	case "ratelimitpolicies":
		return validateRateLimitPolicy(spec)
	default:
		return nil
	}
}

func validateGateway(spec map[string]any) []FieldError {
	errs := []FieldError{}
	if strings.TrimSpace(asString(spec["gatewayClassName"])) == "" {
		errs = append(errs, FieldError{Field: "spec.gatewayClassName", Message: "is required"})
	}
	listeners := asSlice(spec["listeners"])
	if len(listeners) == 0 {
		errs = append(errs, FieldError{Field: "spec.listeners", Message: "must contain at least one listener"})
		return errs
	}
	seenNames := map[string]bool{}
	for i, l := range listeners {
		m, ok := l.(map[string]any)
		if !ok {
			errs = append(errs, FieldError{Field: fmt.Sprintf("spec.listeners[%d]", i), Message: "must be an object"})
			continue
		}
		name := strings.TrimSpace(asString(m["name"]))
		if name == "" {
			errs = append(errs, FieldError{Field: fmt.Sprintf("spec.listeners[%d].name", i), Message: "is required"})
		} else if seenNames[name] {
			errs = append(errs, FieldError{Field: fmt.Sprintf("spec.listeners[%d].name", i), Message: "must be unique"})
		} else {
			seenNames[name] = true
		}
		if strings.TrimSpace(asString(m["protocol"])) == "" {
			errs = append(errs, FieldError{Field: fmt.Sprintf("spec.listeners[%d].protocol", i), Message: "is required"})
		}
		port, ok := asInt(m["port"])
		if !ok || port < 1 || port > 65535 {
			errs = append(errs, FieldError{Field: fmt.Sprintf("spec.listeners[%d].port", i), Message: "must be in range 1..65535"})
		}
	}
	return errs
}

func validateHTTPRoute(spec map[string]any) []FieldError {
	errs := []FieldError{}
	if len(asSlice(spec["parentRefs"])) == 0 {
		errs = append(errs, FieldError{Field: "spec.parentRefs", Message: "must contain at least one parentRef"})
	}
	rules := asSlice(spec["rules"])
	if len(rules) == 0 {
		errs = append(errs, FieldError{Field: "spec.rules", Message: "must contain at least one rule"})
		return errs
	}
	for i, r := range rules {
		rm, ok := r.(map[string]any)
		if !ok {
			errs = append(errs, FieldError{Field: fmt.Sprintf("spec.rules[%d]", i), Message: "must be an object"})
			continue
		}
		backs := asSlice(rm["backendRefs"])
		if len(backs) == 0 {
			errs = append(errs, FieldError{Field: fmt.Sprintf("spec.rules[%d].backendRefs", i), Message: "must contain at least one backendRef"})
		}
		for j, b := range backs {
			bm, ok := b.(map[string]any)
			if !ok {
				errs = append(errs, FieldError{Field: fmt.Sprintf("spec.rules[%d].backendRefs[%d]", i, j), Message: "must be an object"})
				continue
			}
			if strings.TrimSpace(asString(bm["name"])) == "" {
				errs = append(errs, FieldError{Field: fmt.Sprintf("spec.rules[%d].backendRefs[%d].name", i, j), Message: "is required"})
			}
			if port, ok := asInt(bm["port"]); !ok || port < 1 || port > 65535 {
				errs = append(errs, FieldError{Field: fmt.Sprintf("spec.rules[%d].backendRefs[%d].port", i, j), Message: "must be in range 1..65535"})
			}
		}
	}
	return errs
}

func validateAuthPolicy(spec map[string]any) []FieldError {
	errs := []FieldError{}
	target, ok := spec["targetRef"].(map[string]any)
	if !ok {
		return []FieldError{{Field: "spec.targetRef", Message: "is required and must be an object"}}
	}
	if strings.TrimSpace(asString(target["kind"])) == "" {
		errs = append(errs, FieldError{Field: "spec.targetRef.kind", Message: "is required"})
	}
	if strings.TrimSpace(asString(target["name"])) == "" {
		errs = append(errs, FieldError{Field: "spec.targetRef.name", Message: "is required"})
	}
	if rules, exists := spec["rules"]; exists {
		if _, ok := rules.(map[string]any); !ok {
			errs = append(errs, FieldError{Field: "spec.rules", Message: "must be an object"})
		}
	}
	return errs
}

func validateRateLimitPolicy(spec map[string]any) []FieldError {
	errs := []FieldError{}
	target, ok := spec["targetRef"].(map[string]any)
	if !ok {
		errs = append(errs, FieldError{Field: "spec.targetRef", Message: "is required and must be an object"})
	} else {
		if strings.TrimSpace(asString(target["kind"])) == "" {
			errs = append(errs, FieldError{Field: "spec.targetRef.kind", Message: "is required"})
		}
		if strings.TrimSpace(asString(target["name"])) == "" {
			errs = append(errs, FieldError{Field: "spec.targetRef.name", Message: "is required"})
		}
	}
	limits, ok := spec["limits"].(map[string]any)
	if !ok || len(limits) == 0 {
		errs = append(errs, FieldError{Field: "spec.limits", Message: "must contain at least one named limit"})
		return errs
	}
	for limitName, raw := range limits {
		lm, ok := raw.(map[string]any)
		if !ok {
			errs = append(errs, FieldError{Field: "spec.limits." + limitName, Message: "must be an object"})
			continue
		}
		rates := asSlice(lm["rates"])
		if len(rates) == 0 {
			errs = append(errs, FieldError{Field: "spec.limits." + limitName + ".rates", Message: "must contain at least one rate"})
			continue
		}
		for i, rate := range rates {
			rm, ok := rate.(map[string]any)
			if !ok {
				errs = append(errs, FieldError{Field: fmt.Sprintf("spec.limits.%s.rates[%d]", limitName, i), Message: "must be an object"})
				continue
			}
			limit, ok := asInt(rm["limit"])
			if !ok || limit <= 0 {
				errs = append(errs, FieldError{Field: fmt.Sprintf("spec.limits.%s.rates[%d].limit", limitName, i), Message: "must be greater than 0"})
			}
			window := strings.TrimSpace(asString(rm["window"]))
			if window == "" || !rateWindowRe.MatchString(window) {
				errs = append(errs, FieldError{Field: fmt.Sprintf("spec.limits.%s.rates[%d].window", limitName, i), Message: "must match rate window format like 1m, 10s, 1h30m"})
			}
		}
	}
	return errs
}

func asSlice(v any) []any {
	if out, ok := v.([]any); ok {
		return out
	}
	return nil
}

func asString(v any) string {
	if out, ok := v.(string); ok {
		return out
	}
	return ""
}

func asInt(v any) (int, bool) {
	switch vv := v.(type) {
	case int:
		return vv, true
	case int32:
		return int(vv), true
	case int64:
		return int(vv), true
	case float64:
		return int(vv), true
	case string:
		n, err := strconv.Atoi(vv)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

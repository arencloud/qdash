package kube

import "testing"

func TestBuildNamespaceLabelsIncludesInstanceAndProfileLabels(t *testing.T) {
	labels, err := BuildNamespaceLabels("canary", "strict-mtls")
	if err != nil {
		t.Fatalf("build labels: %v", err)
	}
	if labels["istio.io/rev"] != "canary" {
		t.Fatalf("expected canary revision label, got %q", labels["istio.io/rev"])
	}
	if labels["istio-injection"] != "enabled" {
		t.Fatalf("expected istio-injection enabled, got %q", labels["istio-injection"])
	}
	if labels["security.istio.io/tlsMode"] != "istio" {
		t.Fatalf("expected strict mtls label, got %q", labels["security.istio.io/tlsMode"])
	}
}

func TestBuildNamespaceLabelsRejectsUnknownValues(t *testing.T) {
	if _, err := BuildNamespaceLabels("unknown", "default"); err == nil {
		t.Fatalf("expected unknown instance error")
	}
	if _, err := BuildNamespaceLabels("default", "unknown"); err == nil {
		t.Fatalf("expected unknown profile error")
	}
}

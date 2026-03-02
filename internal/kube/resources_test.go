package kube

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

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

func TestDiscoverIstioInstanceConfigsFromSailOperator(t *testing.T) {
	scheme := runtime.NewScheme()
	dynamicClient := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		{Group: "sailoperator.io", Version: "v1", Resource: "istios"}:            "IstioList",
		{Group: "sailoperator.io", Version: "v1", Resource: "istiorevisions"}:    "IstioRevisionList",
		{Group: "sailoperator.io", Version: "v1", Resource: "istiorevisiontags"}: "IstioRevisionTagList",
	}, &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "sailoperator.io/v1",
			"kind":       "Istio",
			"metadata": map[string]any{
				"name": "local",
			},
			"spec": map[string]any{
				"values": map[string]any{
					"meshConfig": map[string]any{
						"discoverySelectors": []any{
							map[string]any{
								"matchLabels": map[string]any{
									"local-discovery": "enabled",
								},
							},
						},
					},
				},
			},
		},
	}, &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "sailoperator.io/v1",
			"kind":       "IstioRevision",
			"metadata": map[string]any{
				"name": "local-v1-27-5",
				"ownerReferences": []any{
					map[string]any{
						"kind": "Istio",
						"name": "local",
					},
				},
			},
			"spec": map[string]any{
				"values": map[string]any{
					"pilot": map[string]any{
						"revision": "local-v1-27-5",
					},
				},
			},
		},
	}, &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "sailoperator.io/v1",
			"kind":       "IstioRevisionTag",
			"metadata": map[string]any{
				"name": "local",
			},
			"spec": map[string]any{
				"targetRef": map[string]any{
					"name": "local-v1-27-5",
				},
			},
		},
	})

	rs := NewResourceService(&Client{
		Core:    k8sfake.NewSimpleClientset(),
		Dynamic: dynamicClient,
	})
	configs, err := rs.DiscoverIstioInstanceConfigs(context.Background())
	if err != nil {
		t.Fatalf("discover configs: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected one config, got %d", len(configs))
	}
	cfg := configs[0]
	if cfg.DiscoveryLabel != "local-discovery=enabled" {
		t.Fatalf("expected local discovery label, got %q", cfg.DiscoveryLabel)
	}
	if !contains(cfg.RevisionTags, "local-v1-27-5") {
		t.Fatalf("expected revision local-v1-27-5, got %+v", cfg.RevisionTags)
	}
	if !contains(cfg.RevisionTags, "local") {
		t.Fatalf("expected tag local, got %+v", cfg.RevisionTags)
	}
	if cfg.InstanceName != "local" {
		t.Fatalf("expected instance local, got %q", cfg.InstanceName)
	}
}

func contains(values []string, wanted string) bool {
	for _, v := range values {
		if v == wanted {
			return true
		}
	}
	return false
}

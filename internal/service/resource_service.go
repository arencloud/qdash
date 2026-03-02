package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/egevorky/qdash/internal/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceService struct {
	krs *kube.ResourceService
}

func NewResourceService(krs *kube.ResourceService) *ResourceService {
	return &ResourceService{krs: krs}
}

func (s *ResourceService) UpsertGeneric(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, spec map[string]any) error {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": fmt.Sprintf("%s/%s", gvr.Group, gvr.Version),
		"kind":       kindFromResource(gvr.Resource),
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}}
	return s.krs.Upsert(ctx, gvr, namespace, obj)
}

func (s *ResourceService) List(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]unstructured.Unstructured, error) {
	return s.krs.List(ctx, gvr, namespace)
}

func (s *ResourceService) Delete(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) error {
	return s.krs.Delete(ctx, gvr, namespace, name)
}

func (s *ResourceService) ListGatewayClasses(ctx context.Context) ([]string, error) {
	return s.krs.ListGatewayClasses(ctx)
}

func (s *ResourceService) CreateNamespace(ctx context.Context, name, instance, profile string, extraLabels map[string]string) error {
	labels, err := kube.BuildNamespaceLabels(instance, profile)
	if err != nil {
		return err
	}
	for k, v := range extraLabels {
		if strings.TrimSpace(k) == "" {
			continue
		}
		labels[k] = v
	}
	return s.krs.CreateNamespace(ctx, name, labels)
}

func (s *ResourceService) NamespaceExists(ctx context.Context, name string) (bool, error) {
	return s.krs.NamespaceExists(ctx, name)
}

func NamespaceProfiles() []string {
	profiles := make([]string, 0, len(kube.NamespaceIstioProfiles()))
	for name := range kube.NamespaceIstioProfiles() {
		profiles = append(profiles, name)
	}
	return profiles
}

func NamespaceInstances() []string {
	instances := make([]string, 0, len(kube.NamespaceIstioInstances()))
	for name := range kube.NamespaceIstioInstances() {
		instances = append(instances, name)
	}
	return instances
}

func kindFromResource(resource string) string {
	switch resource {
	case "gateways":
		return "Gateway"
	case "httproutes":
		return "HTTPRoute"
	case "authpolicies":
		return "AuthPolicy"
	case "ratelimitpolicies":
		return "RateLimitPolicy"
	default:
		return "Unknown"
	}
}

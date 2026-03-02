package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceService struct {
	client *Client
}

func NewResourceService(client *Client) *ResourceService {
	return &ResourceService{client: client}
}

func GatewayGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
}

func HTTPRouteGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
}

func AuthPolicyGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "kuadrant.io", Version: "v1", Resource: "authpolicies"}
}

func RateLimitPolicyGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "kuadrant.io", Version: "v1", Resource: "ratelimitpolicies"}
}

func (s *ResourceService) List(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]unstructured.Unstructured, error) {
	list, err := s.client.Dynamic.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *ResourceService) Upsert(ctx context.Context, gvr schema.GroupVersionResource, namespace string, obj *unstructured.Unstructured) error {
	res := s.client.Dynamic.Resource(gvr).Namespace(namespace)
	_, err := res.Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err == nil {
		_, updateErr := res.Update(ctx, obj, metav1.UpdateOptions{})
		return updateErr
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	_, createErr := res.Create(ctx, obj, metav1.CreateOptions{})
	return createErr
}

func (s *ResourceService) Delete(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) error {
	return s.client.Dynamic.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (s *ResourceService) ListGatewayClasses(ctx context.Context) ([]string, error) {
	gvr := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}
	list, err := s.client.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, item.GetName())
	}
	return out, nil
}

func (s *ResourceService) CreateNamespace(ctx context.Context, name string, labels map[string]string) error {
	_, err := s.client.Core.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
	}, metav1.CreateOptions{})
	return err
}

func (s *ResourceService) NamespaceExists(ctx context.Context, name string) (bool, error) {
	_, err := s.client.Core.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func NamespaceIstioInstances() map[string]map[string]string {
	return map[string]map[string]string{
		"default": {
			"istio.io/rev": "default",
		},
		"canary": {
			"istio.io/rev": "canary",
		},
		"ambient": {
			"istio.io/rev": "ambient",
		},
	}
}

func NamespaceIstioProfiles() map[string]map[string]string {
	return map[string]map[string]string{
		"default": {
			"istio-injection": "enabled",
		},
		"ambient": {
			"istio.io/dataplane-mode": "ambient",
		},
		"strict-mtls": {
			"istio-injection":           "enabled",
			"security.istio.io/tlsMode": "istio",
		},
	}
}

func BuildNamespaceLabels(instance, profile string) (map[string]string, error) {
	instance = stringDefault(instance, "default")
	profile = stringDefault(profile, "default")

	instances := NamespaceIstioInstances()
	base, ok := instances[instance]
	if !ok {
		return nil, fmt.Errorf("unknown instance: %s", instance)
	}
	allProfiles := NamespaceIstioProfiles()
	profileLabels, ok := allProfiles[profile]
	if !ok {
		return nil, fmt.Errorf("unknown profile: %s", profile)
	}

	out := make(map[string]string, len(base)+len(profileLabels))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range profileLabels {
		out[k] = v
	}
	return out, nil
}

func stringDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

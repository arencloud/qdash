package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceService struct {
	client *Client
}

type IstioInstanceConfig struct {
	DiscoveryLabel   string
	RevisionTags     []string
	AdditionalLabels []string
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

func (s *ResourceService) DiscoverIstioLabels(ctx context.Context) ([]string, []string, error) {
	configs, err := s.DiscoverIstioInstanceConfigs(ctx)
	if err != nil {
		return nil, nil, err
	}
	discovery := make([]string, 0, len(configs))
	revSet := map[string]bool{}
	for _, cfg := range configs {
		discovery = append(discovery, cfg.DiscoveryLabel)
		for _, rev := range cfg.RevisionTags {
			revSet[rev] = true
		}
	}
	revisions := make([]string, 0, len(revSet))
	for rev := range revSet {
		revisions = append(revisions, rev)
	}
	sort.Strings(discovery)
	sort.Strings(revisions)
	return discovery, revisions, nil
}

func (s *ResourceService) DiscoverIstioInstanceConfigs(ctx context.Context) ([]IstioInstanceConfig, error) {
	discoverySet := map[string]bool{}
	revSet := map[string]bool{}
	revisionsByDiscovery := map[string]map[string]bool{}
	extraLabelsByRevision := map[string]map[string]bool{}

	nsList, err := s.client.Core.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, ns := range nsList.Items {
		v := strings.TrimSpace(ns.Labels["istio-discovery"])
		rev := strings.TrimSpace(ns.Labels["istio.io/rev"])
		if v != "" {
			discoverySet[v] = true
			if revisionsByDiscovery[v] == nil {
				revisionsByDiscovery[v] = map[string]bool{}
			}
		}
		if rev != "" {
			revSet[rev] = true
			if v != "" {
				revisionsByDiscovery[v][rev] = true
			}
		}
	}

	// Best-effort enrich from Istio mutating webhook objects (includes revision tags in names).
	mwList, mwErr := s.client.Core.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if mwErr == nil {
		for _, mw := range mwList.Items {
			if rev := strings.TrimSpace(mw.Labels["istio.io/rev"]); rev != "" {
				revSet[rev] = true
			}
			if strings.HasPrefix(mw.Name, "istio-revision-tag-") {
				tag := strings.TrimSpace(strings.TrimPrefix(mw.Name, "istio-revision-tag-"))
				if tag != "" {
					revSet[tag] = true
				}
			}
			for _, webhook := range mw.Webhooks {
				if webhook.NamespaceSelector == nil {
					continue
				}
				selectorLabels := selectorLabelOptions(webhook.NamespaceSelector.MatchLabels, webhook.NamespaceSelector.MatchExpressions)
				if len(selectorLabels) == 0 {
					continue
				}
				selectedRevisions := selectorRevisions(webhook.NamespaceSelector.MatchLabels, webhook.NamespaceSelector.MatchExpressions)
				if len(selectedRevisions) == 0 {
					continue
				}
				selectedDiscoveries := selectorDiscoveries(webhook.NamespaceSelector.MatchLabels, webhook.NamespaceSelector.MatchExpressions)
				for _, discovery := range selectedDiscoveries {
					discoverySet[discovery] = true
					if revisionsByDiscovery[discovery] == nil {
						revisionsByDiscovery[discovery] = map[string]bool{}
					}
					for _, rev := range selectedRevisions {
						revisionsByDiscovery[discovery][rev] = true
					}
				}
				for _, rev := range selectedRevisions {
					if extraLabelsByRevision[rev] == nil {
						extraLabelsByRevision[rev] = map[string]bool{}
					}
					for _, label := range selectorLabels {
						if strings.HasPrefix(label, "istio.io/rev=") || strings.HasPrefix(label, "istio-discovery=") {
							continue
						}
						extraLabelsByRevision[rev][label] = true
					}
				}
			}
		}
	}

	if len(discoverySet) == 0 {
		discoverySet["default"] = true
	}
	if len(revSet) == 0 {
		revSet["default"] = true
	}

	allRevisions := make([]string, 0, len(revSet))
	for v := range revSet {
		allRevisions = append(allRevisions, v)
	}
	sort.Strings(allRevisions)

	configs := make([]IstioInstanceConfig, 0, len(discoverySet))
	for discovery := range discoverySet {
		revCandidates := revisionsByDiscovery[discovery]
		revisions := make([]string, 0)
		if len(revCandidates) == 0 {
			revisions = append(revisions, allRevisions...)
		} else {
			for rev := range revCandidates {
				revisions = append(revisions, rev)
			}
			sort.Strings(revisions)
		}

		extraSet := map[string]bool{}
		for _, rev := range revisions {
			for label := range extraLabelsByRevision[rev] {
				extraSet[label] = true
			}
		}
		extras := make([]string, 0, len(extraSet))
		for label := range extraSet {
			extras = append(extras, label)
		}
		sort.Strings(extras)

		configs = append(configs, IstioInstanceConfig{
			DiscoveryLabel:   discovery,
			RevisionTags:     revisions,
			AdditionalLabels: extras,
		})
	}
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].DiscoveryLabel < configs[j].DiscoveryLabel
	})
	return configs, nil
}

func selectorLabelOptions(matchLabels map[string]string, expressions []metav1.LabelSelectorRequirement) []string {
	set := map[string]bool{}
	for k, v := range matchLabels {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		set[k+"="+v] = true
	}
	for _, expr := range expressions {
		if expr.Operator != metav1.LabelSelectorOpIn || len(expr.Values) != 1 {
			continue
		}
		key := strings.TrimSpace(expr.Key)
		val := strings.TrimSpace(expr.Values[0])
		if key == "" || val == "" {
			continue
		}
		set[key+"="+val] = true
	}
	out := make([]string, 0, len(set))
	for label := range set {
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func selectorRevisions(matchLabels map[string]string, expressions []metav1.LabelSelectorRequirement) []string {
	set := map[string]bool{}
	if rev := strings.TrimSpace(matchLabels["istio.io/rev"]); rev != "" {
		set[rev] = true
	}
	for _, expr := range expressions {
		if strings.TrimSpace(expr.Key) != "istio.io/rev" || expr.Operator != metav1.LabelSelectorOpIn {
			continue
		}
		for _, v := range expr.Values {
			rev := strings.TrimSpace(v)
			if rev != "" {
				set[rev] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for rev := range set {
		out = append(out, rev)
	}
	sort.Strings(out)
	return out
}

func selectorDiscoveries(matchLabels map[string]string, expressions []metav1.LabelSelectorRequirement) []string {
	set := map[string]bool{}
	if discovery := strings.TrimSpace(matchLabels["istio-discovery"]); discovery != "" {
		set[discovery] = true
	}
	for _, expr := range expressions {
		if strings.TrimSpace(expr.Key) != "istio-discovery" || expr.Operator != metav1.LabelSelectorOpIn {
			continue
		}
		for _, v := range expr.Values {
			discovery := strings.TrimSpace(v)
			if discovery != "" {
				set[discovery] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for discovery := range set {
		out = append(out, discovery)
	}
	sort.Strings(out)
	return out
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

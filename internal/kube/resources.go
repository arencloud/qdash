package kube

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type ResourceService struct {
	client *Client
}

type IstioInstanceConfig struct {
	InstanceName     string
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

func sailIstioGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "sailoperator.io", Version: "v1", Resource: "istios"}
}

func sailIstioRevisionGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "sailoperator.io", Version: "v1", Resource: "istiorevisions"}
}

func sailIstioRevisionTagGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "sailoperator.io", Version: "v1", Resource: "istiorevisiontags"}
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
	instanceByDiscovery := map[string]string{}
	extraLabelsByRevision := map[string]map[string]bool{}

	s.discoverFromSailOperator(ctx, discoverySet, revSet, revisionsByDiscovery, instanceByDiscovery)

	nsList, err := s.client.Core.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, ns := range nsList.Items {
		rev := strings.TrimSpace(ns.Labels["istio.io/rev"])
		discoveries := namespaceDiscoveryLabels(ns.Labels)
		for _, kv := range discoveries {
			discoverySet[kv] = true
			if revisionsByDiscovery[kv] == nil {
				revisionsByDiscovery[kv] = map[string]bool{}
			}
			if rev != "" {
				revisionsByDiscovery[kv][rev] = true
			}
		}
		if rev != "" {
			revSet[rev] = true
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
		discoverySet["istio-discovery=default"] = true
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
			InstanceName:     instanceByDiscovery[discovery],
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

func (s *ResourceService) discoverFromSailOperator(
	ctx context.Context,
	discoverySet map[string]bool,
	revSet map[string]bool,
	revisionsByDiscovery map[string]map[string]bool,
	instanceByDiscovery map[string]string,
) {
	if s.client == nil || s.client.Dynamic == nil {
		return
	}
	istiorevisions, err := listDynamicSafe(ctx, s.client.Dynamic, sailIstioRevisionGVR())
	if err != nil {
		return
	}
	revisionByInstance := map[string][]string{}
	revisionNameToInstance := map[string]string{}
	for _, item := range istiorevisions.Items {
		revisionName := strings.TrimSpace(item.GetName())
		if revisionName == "" {
			continue
		}
		revision := strings.TrimSpace(sailRevisionValue(item))
		if revision == "" {
			revision = revisionName
		}
		revSet[revision] = true
		instance := strings.TrimSpace(sailOwnerIstioName(item))
		if instance == "" {
			continue
		}
		revisionByInstance[instance] = append(revisionByInstance[instance], revision)
		revisionNameToInstance[revisionName] = instance
	}

	tagsByInstance := map[string][]string{}
	revisionTags, err := listDynamicSafe(ctx, s.client.Dynamic, sailIstioRevisionTagGVR())
	if err == nil {
		for _, tag := range revisionTags.Items {
			tagName := strings.TrimSpace(tag.GetName())
			targetRevision := strings.TrimSpace(sailRevisionTagTarget(tag))
			if tagName == "" || targetRevision == "" {
				continue
			}
			instance := revisionNameToInstance[targetRevision]
			if instance == "" {
				continue
			}
			revSet[tagName] = true
			tagsByInstance[instance] = append(tagsByInstance[instance], tagName)
		}
	}

	istios, err := listDynamicSafe(ctx, s.client.Dynamic, sailIstioGVR())
	if err != nil {
		return
	}
	for _, item := range istios.Items {
		instance := strings.TrimSpace(item.GetName())
		if instance == "" {
			continue
		}
		discoveryLabels := sailDiscoveryLabels(item)
		for _, discovery := range discoveryLabels {
			discoverySet[discovery] = true
			if revisionsByDiscovery[discovery] == nil {
				revisionsByDiscovery[discovery] = map[string]bool{}
			}
			instanceByDiscovery[discovery] = instance
			for _, rev := range revisionByInstance[instance] {
				revisionsByDiscovery[discovery][rev] = true
			}
			for _, tag := range tagsByInstance[instance] {
				revisionsByDiscovery[discovery][tag] = true
			}
		}
	}
}

func sailOwnerIstioName(item unstructured.Unstructured) string {
	owners := item.GetOwnerReferences()
	for _, owner := range owners {
		if strings.EqualFold(strings.TrimSpace(owner.Kind), "Istio") {
			return strings.TrimSpace(owner.Name)
		}
	}
	return ""
}

func sailRevisionValue(item unstructured.Unstructured) string {
	if rev, ok, _ := unstructured.NestedString(item.Object, "spec", "values", "pilot", "revision"); ok {
		return rev
	}
	if rev, ok, _ := unstructured.NestedString(item.Object, "status", "revision"); ok {
		return rev
	}
	return ""
}

func sailRevisionTagTarget(item unstructured.Unstructured) string {
	if name, ok, _ := unstructured.NestedString(item.Object, "spec", "targetRef", "name"); ok {
		return name
	}
	return ""
}

func sailDiscoveryLabels(item unstructured.Unstructured) []string {
	set := map[string]bool{}
	discoverySelectors, ok, _ := unstructured.NestedSlice(item.Object, "spec", "values", "meshConfig", "discoverySelectors")
	if !ok {
		return nil
	}
	for _, selector := range discoverySelectors {
		obj, ok := selector.(map[string]any)
		if !ok {
			continue
		}
		matchLabels, ok := obj["matchLabels"].(map[string]any)
		if !ok {
			continue
		}
		for k, raw := range matchLabels {
			key := strings.TrimSpace(k)
			val := strings.TrimSpace(fmt.Sprint(raw))
			if key == "" || val == "" {
				continue
			}
			set[key+"="+val] = true
		}
	}
	out := make([]string, 0, len(set))
	for label := range set {
		out = append(out, label)
	}
	sort.Strings(out)
	return out
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
	for key, raw := range matchLabels {
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(raw)
		if k == "" || v == "" {
			continue
		}
		if k == "istio-discovery" || strings.HasSuffix(k, "-discovery") {
			set[k+"="+v] = true
		}
	}
	for _, expr := range expressions {
		key := strings.TrimSpace(expr.Key)
		if (key != "istio-discovery" && !strings.HasSuffix(key, "-discovery")) || expr.Operator != metav1.LabelSelectorOpIn {
			continue
		}
		for _, v := range expr.Values {
			discovery := strings.TrimSpace(v)
			if discovery != "" {
				set[key+"="+discovery] = true
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

func namespaceDiscoveryLabels(labels map[string]string) []string {
	set := map[string]bool{}
	for key, val := range labels {
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(val)
		if k == "" || v == "" {
			continue
		}
		if k == "istio-discovery" || strings.HasSuffix(k, "-discovery") {
			set[k+"="+v] = true
		}
	}
	out := make([]string, 0, len(set))
	for kv := range set {
		out = append(out, kv)
	}
	sort.Strings(out)
	return out
}

func listDynamicSafe(ctx context.Context, dynInterface dynamic.Interface, gvr schema.GroupVersionResource) (list *unstructured.UnstructuredList, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("dynamic list panic recovered")
		}
	}()
	return dynInterface.Resource(gvr).List(ctx, metav1.ListOptions{})
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

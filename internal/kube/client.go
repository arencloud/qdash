package kube

import (
	"fmt"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Client struct {
	Core    kubernetes.Interface
	Dynamic dynamic.Interface
}

func NewClient(kubeconfig string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = outOfClusterConfig(kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("load kubernetes config: %w", err)
		}
	}

	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("core client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	return &Client{Core: core, Dynamic: dyn}, nil
}

func outOfClusterConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	home := homedir.HomeDir()
	if home == "" {
		return nil, fmt.Errorf("kubeconfig not provided and HOME is empty")
	}
	return clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config"))
}

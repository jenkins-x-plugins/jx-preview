package previews

import (
	"fmt"

	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
)

// LazyCreatePreviewClientAndNamespace lazy creates the preview client and/or the current namespace if not already defined
func LazyCreatePreviewClientAndNamespace(client versioned.Interface, ns string) (versioned.Interface, string, error) {
	if client != nil && ns != "" {
		return client, ns, nil
	}
	f := kubeclient.NewFactory()
	cfg, err := f.CreateKubeConfig()
	if err != nil {
		return client, ns, fmt.Errorf("failed to get kubernetes config: %w", err)
	}
	client, err = versioned.NewForConfig(cfg)
	if err != nil {
		return client, ns, fmt.Errorf("error building kubernetes clientset: %w", err)
	}
	if ns == "" {
		ns, err = kubeclient.CurrentNamespace()
		if err != nil {
			return client, ns, fmt.Errorf("failed to get current kubernetes namespace: %w", err)
		}
	}
	return client, ns, nil
}

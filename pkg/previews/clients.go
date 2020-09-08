package previews

import (
	"github.com/jenkins-x/jx-kube-client/pkg/kubeclient"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
)

// LazyCreatePreviewClientAndNamespace lazy creates the preview client and/or the current namespace if not already defined
func LazyCreatePreviewClientAndNamespace(client versioned.Interface, ns string) (versioned.Interface, string, error) {
	if client != nil && ns != "" {
		return client, ns, nil
	}
	f := kubeclient.NewFactory()
	cfg, err := f.CreateKubeConfig()
	if err != nil {
		return client, ns, errors.Wrap(err, "failed to get kubernetes config")
	}
	client, err = versioned.NewForConfig(cfg)
	if err != nil {
		return client, ns, errors.Wrap(err, "error building kubernetes clientset")
	}
	if ns == "" {
		ns, err = kubeclient.CurrentNamespace()
		if err != nil {
			return client, ns, errors.Wrap(err, "failed to get current kubernetes namespace")
		}
	}
	return client, ns, nil
}

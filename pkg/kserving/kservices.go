package kserving

import (
	"context"

	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	kserve "knative.dev/serving/pkg/client/clientset/versioned"
)

// FindServiceURL finds the service URL for the given knative service name
func FindServiceURL(ctx context.Context, client kserve.Interface, kubeClient kubernetes.Interface, namespace, name string) (string, *v1.Service, error) {
	if client == nil {
		return "", nil, nil
	}
	svc, err := client.ServingV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", svc, err
	}
	answer := GetServiceURL(ctx, svc, kubeClient, namespace)
	return answer, svc, nil
}

// GetServiceURL returns the URL for the given knative service
func GetServiceURL(ctx context.Context, service *v1.Service, kubeClient kubernetes.Interface, namespace string) string {
	if service == nil {
		return ""
	}
	if service.Status.URL != nil {
		return service.Status.URL.String()
	}
	return ""
	/* TODO old v1alpha1 code before we had a URL
	domain := service.Status.DeprecatedDomain
	if domain == "" {
		if service.Status.Address != nil {
			domain = service.Status.Address.Hostname
		}
	}
	if domain == "" {
		return ""
	}

	name := service.Status.LatestReadyRevisionName
	if name == "" {
		name = service.Status.LatestCreatedRevisionName
	}
	scheme := "http://"
	if name != "" {
		name = name + "-service"
		// lets find the service to determine if its https or http
		svc, err := kubeClient.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil && svc != nil {
			for _, port := range svc.Spec.Ports {
				if port.Port == 443 {
					scheme = "https://"
				}
			}
		}
	}
	return scheme + domain
	*/
}

// LazyCreateKServeClient lazy creates the kserve client if its not defined
func LazyCreateKServeClient(client kserve.Interface) (kserve.Interface, error) {
	if client != nil {
		return client, nil
	}
	f := kubeclient.NewFactory()
	cfg, err := f.CreateKubeConfig()
	if err != nil {
		return client, errors.Wrap(err, "failed to get kubernetes config")
	}
	client, err = kserve.NewForConfig(cfg)
	if err != nil {
		return client, errors.Wrap(err, "error building jx clientset")
	}
	return client, nil
}

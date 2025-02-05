package kserving

import (
	"context"
	"fmt"

	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	kserve "knative.dev/serving/pkg/client/clientset/versioned"
)

// FindServiceURL finds the service URL for the given knative service name
func FindServiceURL(ctx context.Context, client kserve.Interface, namespace, name string) (string, *v1.Service, error) {
	if client == nil {
		return "", nil, nil
	}
	svc, err := client.ServingV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", svc, err
	}
	answer := GetServiceURL(svc)
	return answer, svc, nil
}

// GetServiceURL returns the URL for the given knative service
func GetServiceURL(service *v1.Service) string {
	if service == nil {
		return ""
	}
	if service.Status.URL != nil {
		return service.Status.URL.String()
	}
	return ""
}

// LazyCreateKServeClient lazy creates the kserve client if its not defined
func LazyCreateKServeClient(client kserve.Interface) (kserve.Interface, error) {
	if client != nil {
		return client, nil
	}
	f := kubeclient.NewFactory()
	cfg, err := f.CreateKubeConfig()
	if err != nil {
		return client, fmt.Errorf("failed to get kubernetes config: %w", err)
	}
	client, err = kserve.NewForConfig(cfg)
	if err != nil {
		return client, fmt.Errorf("error building jx clientset: %w", err)
	}
	return client, nil
}

package previews

import (
	"context"

	"github.com/jenkins-x-plugins/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetOrCreatePreview lazy creates the preview client and/or the current namespace if not already defined
func GetOrCreatePreview(client versioned.Interface, ns string, pr *scm.PullRequest, destroyCmd *v1alpha1.Command, gitURL, previewNamespace, path string) (*v1alpha1.Preview, bool, error) {
	create := false

	ctx := context.Background()
	previewInterface := client.PreviewV1alpha1().Previews(ns)
	previews, err := previewInterface.List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
	}
	if err != nil {
		return nil, create, errors.Wrapf(err, "failed to list Previews in namespace %s", ns)
	}

	repo := pr.Repository()
	var found *v1alpha1.Preview
	for i := range previews.Items {
		preview := &previews.Items[i]
		if preview.Name == previewNamespace {
			found = preview
			break
		}
	}
	if found == nil {
		create = true
		found = &v1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{
				Name:      previewNamespace,
				Namespace: ns,
			},
		}
	}
	if found.Namespace == "" {
		found.Namespace = ns
	}
	src := &found.Spec.Source
	src.CloneURL = gitURL
	if repo.Link != "" {
		src.URL = repo.Link
	}
	if src.Ref == "" {
		src.Ref = pr.Sha
	}
	if src.Path == "" {
		src.Path = path
	}
	prr := &found.Spec.PullRequest
	prr.LatestCommit = pr.Head.Sha
	if prr.Number <= 0 {
		prr.Number = pr.Number
	}
	if prr.URL == "" {
		prr.URL = pr.Link
	}
	if prr.Owner == "" {
		prr.Owner = repo.Namespace
	}
	if prr.Repository == "" {
		prr.Repository = repo.Name
	}
	if prr.Title == "" {
		prr.Title = pr.Title
	}
	if prr.Description == "" {
		prr.Description = pr.Body
	}
	if previewNamespace != "" {
		found.Spec.Resources.Namespace = previewNamespace
	}
	found.Spec.DestroyCommand = *destroyCmd
	if create {
		found, err = previewInterface.Create(ctx, found, metav1.CreateOptions{})
		if err != nil {
			return found, create, errors.Wrapf(err, "failed to create Preview %s", found.Name)
		}
		return found, create, nil
	}
	found, err = previewInterface.Update(ctx, found, metav1.UpdateOptions{})
	if err != nil {
		return found, create, errors.Wrapf(err, "failed to update Preview %s", found.Name)
	}
	return found, create, nil
}

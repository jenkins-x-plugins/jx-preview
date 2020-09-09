package previews

import (
	"strconv"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/jx-helpers/pkg/kube/naming"
	"github.com/jenkins-x/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetOrCreatePreview lazy creates the preview client and/or the current namespace if not already defined
func GetOrCreatePreview(client versioned.Interface, ns string, pr *scm.PullRequest, destroyCmd v1alpha1.Command, gitURL, previewNamespace, path string) (*v1alpha1.Preview, bool, error) {
	create := false

	previewInterface := client.PreviewV1alpha1().Previews(ns)
	previews, err := previewInterface.List(metav1.ListOptions{})
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
		prs := &preview.Spec.PullRequest
		if prs.Number == pr.Number && prs.Repository == repo.Name && prs.Owner == repo.Namespace {
			found = preview
			break
		}
	}
	if found == nil {
		create = true
		found = &v1alpha1.Preview{
			ObjectMeta: metav1.ObjectMeta{
				Name:      naming.ToValidName(repo.FullName + "-pr-" + strconv.Itoa(pr.Number)),
				Namespace: ns,
			},
		}
	}
	if found.Namespace == "" {
		found.Namespace = ns
	}
	src := &found.Spec.Source
	src.URL = gitURL
	if src.Ref == "" {
		src.Ref = pr.Sha
	}
	if src.Path == "" {
		src.Path = path
	}
	prr := &found.Spec.PullRequest
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
		found.Spec.PreviewNamespace = previewNamespace
	}
	found.Spec.DestroyCommand = destroyCmd
	if create {
		found, err = previewInterface.Create(found)
		if err != nil {
			return found, create, errors.Wrapf(err, "failed to create Preview %s", found.Name)
		}
		return found, create, nil
	}
	found, err = previewInterface.Update(found)
	if err != nil {
		return found, create, errors.Wrapf(err, "failed to update Preview %s", found.Name)
	}
	return found, create, nil
}

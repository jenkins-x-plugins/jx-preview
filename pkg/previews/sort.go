package previews

import (
	"sort"

	"github.com/jenkins-x-plugins/jx-preview/pkg/apis/preview/v1alpha1"
)

// Less compares two previews to sort them in order
func Less(a *v1alpha1.Preview, b *v1alpha1.Preview) bool {
	if a.Spec.PullRequest.Owner < b.Spec.PullRequest.Owner {
		return true
	}
	if a.Spec.PullRequest.Repository < b.Spec.PullRequest.Repository {
		return true
	}
	return a.Spec.PullRequest.Number > b.Spec.PullRequest.Number
}

// SortPreviews sorts the previews in order
func SortPreviews(resources []v1alpha1.Preview) {
	sort.Slice(resources, func(i, j int) bool {
		return Less(&resources[i], &resources[j])
	})

}

package fakepreviews

import (
	"strconv"

	"github.com/jenkins-x/go-scm/scm"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	"github.com/jenkins-x/jx-preview/pkg/apis/preview/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateTestPreviewAndPullRequest creates a fake PullRequest
func CreateTestPreviewAndPullRequest(data *fakescm.Data, ns, owner, repo string, prNumber int) (*v1alpha1.Preview, *scm.PullRequest) {
	prNumberText := strconv.Itoa(prNumber)
	name := owner + "-" + repo + "-" + prNumberText

	repoLink := "https://fake.com/" + owner + "/" + repo
	prTitle := "my PR thingy"
	prBody := "some awesome changes"
	prLink := repoLink + "/pull/" + prNumberText
	sha := "abcdef1234"
	previewNamespace := ns + "-" + owner + "-" + repo + "-pr-" + prNumberText
	pr := &scm.PullRequest{
		Number: prNumber,
		Title:  prTitle,
		Body:   prBody,
		Labels: nil,
		Sha:    sha,
		Base: scm.PullRequestBranch{
			Repo: scm.Repository{
				Namespace: owner,
				Name:      repo,
				Link:      repoLink,
			},
		},
		Head:      scm.PullRequestBranch{},
		Author:    scm.User{},
		Milestone: scm.Milestone{},
		Link:      prLink,
	}

	if data.PullRequests == nil {
		data.PullRequests = map[int]*scm.PullRequest{}
	}
	data.PullRequests[prNumber] = pr

	preview := &v1alpha1.Preview{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1alpha1.PreviewSpec{
			Source: v1alpha1.PreviewSource{
				URL:      repoLink,
				CloneURL: repoLink,
			},
			PullRequest: v1alpha1.PullRequest{
				Number:      prNumber,
				Owner:       owner,
				Repository:  repo,
				URL:         repoLink,
				Title:       prTitle,
				Description: prBody,
			},
			Resources: v1alpha1.Resources{
				Namespace: previewNamespace,
			},
		},
	}

	return preview, pr
}

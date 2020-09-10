package fakescms

import (
	"strconv"

	"github.com/jenkins-x/go-scm/scm"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
)

// CreatePullRequest creates a fake PullRequest for testing
func CreatePullRequest(data *fakescm.Data, owner, repo string, prNumber int) *scm.PullRequest {
	prNumberText := strconv.Itoa(prNumber)

	repoLink := "https://fake.com/" + owner + "/" + repo
	prTitle := "my PR thingy"
	prBody := "some awesome changes"
	prLink := repoLink + "/pull/" + prNumberText
	sha := "abcdef1234"
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
	return pr
}

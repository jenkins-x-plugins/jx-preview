package gc_test

import (
	"strconv"
	"testing"

	"github.com/jenkins-x/go-scm/scm"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner/fakerunner"
	"github.com/jenkins-x/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-preview/pkg/cmd/gc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

func TestPreviewGC(t *testing.T) {
	gitUser := "myuser"
	ns := "jx"

	scmClient, fakeScmData := fakescm.NewDefault()

	preview1, pr1 := createTestPreviewAndPullRequest(fakeScmData, ns, "myower", "myrepo", 2)
	preview2, _ := createTestPreviewAndPullRequest(fakeScmData, ns, "myower", "myrepo", 3)
	preview3, pr3 := createTestPreviewAndPullRequest(fakeScmData, ns, "myower", "another", 1)

	fakeScmData.CurrentUser.Login = gitUser

	previewClient := fake.NewSimpleClientset(preview1, preview2, preview3)
	kubeClient := fakekube.NewSimpleClientset()

	testCases := []struct {
		name            string
		expectedDeleted []string
		initialise      func() error
	}{
		{
			name: "startup",
		},
		{
			name:            "gc1",
			expectedDeleted: []string{preview1.Name},
			initialise: func() error {
				pr1.State = "Closed"
				t.Logf("modified state of PR %s to: %s", pr1.Link, pr1.State)
				return nil
			},
		},
		{
			name: "no-gc-after-gc1",
		},
		{
			name:            "gc3",
			expectedDeleted: []string{preview3.Name},
			initialise: func() error {
				pr3.State = "Merged"
				t.Logf("modified state of PR %s to: %s", pr3.Link, pr3.State)
				return nil
			},
		},
	}

	runner := &fakerunner.FakeRunner{}
	for _, tc := range testCases {
		_, o := gc.NewCmdGCPreviews()

		o.PreviewClient = previewClient
		o.KubeClient = kubeClient
		o.Namespace = ns
		o.ScmClient = scmClient
		o.CommandRunner = runner.Run

		if tc.initialise != nil {
			err := tc.initialise()
			require.NoError(t, err, "failed to initialise test  %s", tc.name)
		}
		t.Logf("running GC for test: %s\n", tc.name)
		err := o.Run()
		require.NoError(t, err, "should not have failed the GC for test %s", tc.name)

		assert.Equal(t, tc.expectedDeleted, o.Deleted, "deleted previews")
	}

	// lets assert there's only 1 Preview left
	previewList, err := previewClient.PreviewV1alpha1().Previews(ns).List(metav1.ListOptions{})
	require.NoError(t, err, "failed to list the remaining previews in ns %s", ns)

	// verify the correct preview is remaining
	remainingPreviews := previewList.Items
	require.Len(t, remainingPreviews, 1, "should have one remaining Preview")
	if assert.Equal(t, preview2.Name, remainingPreviews[0].Name, "wrong remaining preview name") {
		t.Logf("correctly has a single reamining Preview of name %s\n", remainingPreviews[0].Name)
	}

	for _, c := range runner.OrderedCommands {
		t.Logf("fake comamnds: %s\n", c.CLI())
	}
}

func createTestPreviewAndPullRequest(data *fakescm.Data, ns, owner, repo string, prNumber int) (*v1alpha1.Preview, *scm.PullRequest) {
	prNumberText := strconv.Itoa(prNumber)
	name := owner + "-" + repo + "-" + prNumberText

	repoLink := "https://fake.com/" + owner + "/" + repo
	prTitle := "my PR thingy"
	prBody := "some awesome changes"
	prLink := repoLink + "/pull/" + prNumberText
	sha := "abcdef1234"
	previewNamespace := ns + "-" + owner + "-" + repo + "-pr-" + prNumberText

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
			PreviewNamespace: previewNamespace,
		},
	}

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
	return preview, pr
}

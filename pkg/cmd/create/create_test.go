package create_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jenkins-x/go-scm/scm"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner/fakerunner"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-preview/pkg/cmd/destroy"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"

	"github.com/jenkins-x/jx-preview/pkg/cmd/create"
	"github.com/stretchr/testify/require"
)

func TestPreviewCreate(t *testing.T) {
	containerRegistry := "ghcr.io"
	gitUser := "myuser"
	gitToken := "mytoken"
	owner := "myowner"
	repo := "myrepo"
	ns := "jx"
	prNumber := 5
	sha := "abcdef1234"
	previewNamespace := ns + "-" + owner + "-" + repo + "-pr-" + strconv.Itoa(prNumber)

	helmFile := filepath.Join("..", "preview", "helmfile.yaml")
	repoLink := "https://" + gitUser + ":" + gitToken + "@fake.com/" + owner + "/" + repo

	previewClient := fake.NewSimpleClientset()

	scmClient, fakeScmData := fakescm.NewDefault()

	prTitle := "my PR thingy"
	prBody := "some awesome changes"
	prLink := repoLink + "/pull/" + strconv.Itoa(prNumber)
	fakeScmData.PullRequests = map[int]*scm.PullRequest{
		prNumber: {
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
		},
	}

	previewName := ""
	for _, testName := range []string{"create", "update"} {
		_, o := create.NewCmdPreviewCreate()

		o.PreviewClient = previewClient
		o.Namespace = ns
		o.GitToken = "dummy"
		o.SourceURL = repoLink + ".git"
		o.Number = prNumber
		o.PreviewHelmfile = helmFile
		o.DockerRegistry = containerRegistry

		runner := &fakerunner.FakeRunner{}
		o.CommandRunner = runner.Run
		o.ScmClient = scmClient

		err := o.Run()
		require.NoError(t, err, "failed to run command in test %s", testName)

		runner.ExpectResults(t,
			fakerunner.FakeResult{
				CLI: `helmfile --file ../preview/helmfile.yaml sync`,
			},
		)

		previewList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(metav1.ListOptions{})
		require.NoError(t, err, "failed to list previews in namespace %s for test %s", ns, testName)
		require.NotNil(t, previewList, "no preview list returned in namespace %s for test %s", ns, testName)
		require.Len(t, previewList.Items, 1, "previews in namespace %s for test %s", ns, testName)
		preview := previewList.Items[0]

		previewName = preview.Name
		t.Logf("found preview %s in namespace %s for test %s", previewName, ns, testName)

		assert.Equal(t, previewNamespace, preview.Spec.PreviewNamespace, "preview.Spec.PreviewNamespace")

		assert.NotEmpty(t, preview.Spec.DestroyCommand.Args, "preview.Spec.DestroyCommand.Args")
		assert.NotEmpty(t, preview.Spec.DestroyCommand.Env, "preview.Spec.DestroyCommand.Env")

		prs := &preview.Spec.PullRequest
		assert.Equal(t, prNumber, prs.Number, "preview.Spec.PullRequest.Number")
		assert.Equal(t, owner, prs.Owner, "preview.Spec.PullRequest.Owner")
		assert.Equal(t, repo, prs.Repository, "preview.Spec.PullRequest.Repository")
		assert.Equal(t, prTitle, prs.Title, "preview.Spec.PullRequest.Title")
		assert.Equal(t, prBody, prs.Description, "preview.Spec.PullRequest.Description")
		assert.Equal(t, prLink, prs.URL, "preview.Spec.PullRequest.URL")

		prsrc := &preview.Spec.Source
		assert.Equal(t, repoLink, prsrc.URL, "preview.Spec.Source.URL")
		assert.Equal(t, sha, prsrc.Ref, "preview.Spec.Source.Ref")
	}

	// now lets test deleting the preview
	_, do := destroy.NewCmdPreviewDestroy()
	do.PreviewClient = previewClient
	do.Namespace = ns
	do.Name = previewName

	runner := &fakerunner.FakeRunner{}
	do.CommandRunner = runner.Run

	do.KubeClient = fakekube.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: previewNamespace,
			},
		},
	)

	err := do.Run()
	require.NoError(t, err, "failed to delete preview %s", previewName)

	runner.ExpectResults(t,
		fakerunner.FakeResult{
			CLI: `helmfile --file ../preview/helmfile.yaml destroy`,
		},
	)

	// now lets check we removed the preview namespace
	namespaceList, err := do.KubeClient.CoreV1().Namespaces().List(metav1.ListOptions{})
	require.NoError(t, err, "failed to list namespaces")
	require.Len(t, namespaceList.Items, 0, "should not have any Namespaces")
}

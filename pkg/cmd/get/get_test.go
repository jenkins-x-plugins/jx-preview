package get

import (
	"fmt"
	"os"
	"testing"

	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x-plugins/jx-preview/pkg/fakescms"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews/fakepreviews"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	jxfake "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewGet(t *testing.T) {
	scmClient, fakeData := fakescm.NewDefault()

	owner := "owner"
	repo := "test-repo"
	prNumber := 1
	sourceURL := fmt.Sprintf("https://fake.com/%s/%s", owner, repo)
	ns := "jx"
	tmpDir, err := os.MkdirTemp("", "")
	latestCommit := "test-commit-hash"
	require.NoError(t, err, "failed to create temp dir")

	preview1, _ := fakepreviews.CreateTestPreviewAndPullRequest(fakeData, ns, owner, repo, 1)
	preview1.Spec.PullRequest.LatestCommit = latestCommit

	fakescms.CreatePullRequest(fakeData, owner, repo, prNumber)

	previewClient := fake.NewSimpleClientset(preview1)

	devEnv := jxenv.CreateDefaultDevEnvironment(ns)
	devEnv.Namespace = ns
	devEnv.Spec.Source.URL = sourceURL

	jxClient := jxfake.NewSimpleClientset(devEnv)

	testCases := []struct {
		name    string
		current bool
		wait    bool
	}{
		{
			name:    "get",
			current: false,
			wait:    false,
		},
		{
			name:    "get current",
			current: true,
			wait:    false,
		},
		{
			name:    "get current wait",
			current: true,
			wait:    true,
		},
	}

	for _, tc := range testCases {
		_, o := NewCmdGetPreview()

		o.ScmClient = scmClient
		o.PreviewClient = previewClient
		o.JXClient = jxClient
		o.SourceURL = sourceURL
		o.PullRequestOptions.Number = prNumber
		o.Repository = repo
		o.DiscoverFromGit = false
		o.Namespace = ns
		o.Current = tc.current
		o.Dir = tmpDir
		o.LatestCommit = latestCommit

		t.Logf("running get for test: %s", tc.name)
		err := o.Run()
		require.NoError(t, err)

		if tc.current {
			assert.Equal(t, fmt.Sprintf("https://%s-pr%v.mqube-test.com", repo, prNumber), o.OutputEnvVars["PREVIEW_URL"])
			assert.Equal(t, fmt.Sprintf("%s-%s-%v", owner, repo, prNumber), o.OutputEnvVars["PREVIEW_NAME"])
			assert.Equal(t, fmt.Sprintf("%s-%s-%s-pr-%v", ns, owner, repo, prNumber), o.OutputEnvVars["PREVIEW_NAMESPACE"])
			assert.Equal(t, sourceURL, o.OutputEnvVars["PREVIEW_PULL_REQUEST_URL"])
		}
	}
}

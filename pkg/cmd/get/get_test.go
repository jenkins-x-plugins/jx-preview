package get

import (
	"fmt"
	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x-plugins/jx-preview/pkg/fakescms"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews/fakepreviews"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	jxfake "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPreviewGet(t *testing.T) {
	scmClient, fakeData := fakescm.NewDefault()

	owner := "owner"
	repo := "test-repo"
	prNumber := 1
	sourceUrl := fmt.Sprintf("https://fake.com/%s/%s", owner, repo)
	ns := "jx"

	preview1, _ := fakepreviews.CreateTestPreviewAndPullRequest(fakeData, ns, owner, repo, 1)
	fakescms.CreatePullRequest(fakeData, owner, repo, prNumber)

	previewClient := fake.NewSimpleClientset(preview1)

	devEnv := jxenv.CreateDefaultDevEnvironment(ns)
	devEnv.Namespace = ns
	devEnv.Spec.Source.URL = sourceUrl

	jxClient := jxfake.NewSimpleClientset(devEnv)

	testCases := []struct {
		name    string
		current bool
	}{
		{
			name:    "get",
			current: false,
		},
		{
			name:    "get current",
			current: true,
		},
	}

	for _, tc := range testCases {
		_, o := NewCmdGetPreview()

		o.ScmClient = scmClient
		o.PreviewClient = previewClient
		o.JXClient = jxClient
		o.SourceURL = sourceUrl
		o.PullRequestOptions.Number = prNumber
		o.Repository = repo
		o.DiscoverFromGit = false
		o.Namespace = ns
		o.Current = tc.current

		t.Logf("running get for test: %s", tc.name)
		err := o.Run()
		require.NoError(t, err)

		if tc.current {
			assert.Equal(t, fmt.Sprintf("https://%s-pr%v.mqube-test.com", repo, prNumber), o.OutputEnvVars["PREVIEW_URL"])
			assert.Equal(t, fmt.Sprintf("%s-%s-%v", owner, repo, prNumber), o.OutputEnvVars["PREVIEW_NAME"])
			assert.Equal(t, fmt.Sprintf("%s-%s-%s-pr-%v", ns, owner, repo, prNumber), o.OutputEnvVars["PREVIEW_NAMESPACE"])
			assert.Equal(t, sourceUrl, o.OutputEnvVars["PREVIEW_PULL_REQUEST_URL"])
		}
	}
}

package get

import (
	"fmt"
	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x-plugins/jx-preview/pkg/fakescms"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews/fakepreviews"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPreviewGetCurrent(t *testing.T) {
	scmClient, fakeData := fakescm.NewDefault()

	owner := "owner"
	repo := "test-repo"
	prNumber := 1
	sourceUrl := fmt.Sprintf("https://fake.com/%s/%s", owner, repo)
	ns := "jx"

	preview1, _ := fakepreviews.CreateTestPreviewAndPullRequest(fakeData, ns, owner, repo, 1)
	fakescms.CreatePullRequest(fakeData, owner, repo, prNumber)

	previewClient := fake.NewSimpleClientset(preview1)

	_, o := NewCmdGetPreview()

	o.ScmClient = scmClient
	o.PreviewClient = previewClient
	o.SourceURL = sourceUrl
	o.PullRequestOptions.Number = prNumber
	o.Repository = repo
	o.DiscoverFromGit = false
	o.Namespace = ns
	o.Current = true

	err := o.Run()
	require.NoError(t, err)

	assert.Equal(t, fmt.Sprintf("https://%s-pr%v.mqube-test.com", repo, prNumber), o.OutputEnvVars["PREVIEW_URL"])
	assert.Equal(t, fmt.Sprintf("%s-%s-%v", owner, repo, prNumber), o.OutputEnvVars["PREVIEW_NAME"])
	assert.Equal(t, fmt.Sprintf("%s-%s-%s-pr-%v", ns, owner, repo, prNumber), o.OutputEnvVars["PREVIEW_NAMESPACE"])
	assert.Equal(t, sourceUrl, o.OutputEnvVars["PREVIEW_PULL_REQUEST_URL"])
}

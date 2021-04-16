package gc_test

import (
	"context"
	"testing"

	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x-plugins/jx-preview/pkg/cmd/gc"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews/fakepreviews"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	jxfake "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner/fakerunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

func TestPreviewGC(t *testing.T) {
	gitUser := "myuser"
	ns := "jx"

	scmClient, fakeScmData := fakescm.NewDefault()

	preview1, pr1 := fakepreviews.CreateTestPreviewAndPullRequest(fakeScmData, ns, "myower", "myrepo", 2)
	preview2, _ := fakepreviews.CreateTestPreviewAndPullRequest(fakeScmData, ns, "myower", "myrepo", 3)
	preview3, pr3 := fakepreviews.CreateTestPreviewAndPullRequest(fakeScmData, ns, "myower", "another", 1)

	fakeScmData.CurrentUser.Login = gitUser

	previewClient := fake.NewSimpleClientset(preview1, preview2, preview3)
	kubeClient := fakekube.NewSimpleClientset()

	devEnv := jxenv.CreateDefaultDevEnvironment(ns)
	devEnv.Namespace = ns
	devEnv.Spec.Source.URL = "https://github.com/myorg/my-gitops-repo.git"

	jxClient := jxfake.NewSimpleClientset(devEnv)

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

		o.GitUser = "fakeuser"
		o.GitToken = "faketoken"
		o.PreviewClient = previewClient
		o.KubeClient = kubeClient
		o.JXClient = jxClient
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
	ctx := context.Background()
	previewList, err := previewClient.PreviewV1alpha1().Previews(ns).List(ctx, metav1.ListOptions{})
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

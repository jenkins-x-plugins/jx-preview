package gc_test

import (
	"context"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"os"
	"path/filepath"
	"testing"

	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x-plugins/jx-preview/pkg/cmd/gc"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews/fakepreviews"
	"github.com/jenkins-x/go-scm/scm"
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
	preview4, pr4 := fakepreviews.CreateTestPreviewAndPullRequest(fakeScmData, ns, "myower", "athird", 4)

	fakeScmData.CurrentUser.Login = gitUser

	previewClient := fake.NewSimpleClientset(preview1, preview2, preview3, preview4)
	kubeClient := fakekube.NewSimpleClientset()

	devEnv := jxenv.CreateDefaultDevEnvironment(ns)
	devEnv.Namespace = ns
	devEnv.Spec.Source.URL = "https://github.com/myorg/my-gitops-repo.git"

	jxClient := jxfake.NewSimpleClientset(devEnv)

	testCases := []struct {
		name            string
		expectedDeleted []string
		initialise      func(o *gc.Options) error
	}{
		{
			name: "startup",
		},
		{
			name:            "gc1",
			expectedDeleted: []string{preview1.Name},
			initialise: func(_ *gc.Options) error {
				pr1.Closed = true
				return nil
			},
		},
		{
			name: "no-gc-after-gc1",
		},
		{
			name:            "gc3",
			expectedDeleted: []string{preview3.Name},
			initialise: func(_ *gc.Options) error {
				pr3.Merged = true
				return nil
			},
		},
		{
			name: "gc4",
			initialise: func(_ *gc.Options) error {
				pr4.Draft = true
				return nil
			},
		},
		{
			name: "gc5",
			initialise: func(o *gc.Options) error {
				o.DestroyDrafts = true
				pr4.Labels = []*scm.Label{{Name: "ok-to-test"}}
				return nil
			},
		},
		{
			name:            "gc6",
			expectedDeleted: []string{preview4.Name},
			initialise: func(o *gc.Options) error {
				o.DestroyDrafts = true
				pr4.Labels = nil
				return nil
			},
		},
	}

	runner := &fakerunner.FakeRunner{
		CommandRunner: func(c *cmdrunner.Command) (string, error) {
			// git clone
			if c.Name == "git" && c.Args[0] == "clone" {
				err := os.MkdirAll(filepath.Join(c.Args[2], "helmfiles", "jx"), 0755)
				if err != nil {
					return "", err
				}
				err = os.WriteFile(filepath.Join(c.Args[2], "helmfiles", "jx", "jx-values.yaml"), []byte(""), 0755)
				if err != nil {
					return "", err
				}
			}
			return "", nil
		},
	}
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
			err := tc.initialise(o)
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
		t.Logf("correctly has a single remaining Preview of name %s\n", remainingPreviews[0].Name)
	}

	for _, c := range runner.OrderedCommands {
		t.Logf("fake comamnds: %s\n", c.CLI())
	}
}

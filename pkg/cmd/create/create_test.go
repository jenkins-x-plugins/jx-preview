package create_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jenkins-x/go-scm/scm"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	jxfake "github.com/jenkins-x/jx-api/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner/fakerunner"
	"github.com/jenkins-x/jx-helpers/pkg/files"
	"github.com/jenkins-x/jx-helpers/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-preview/pkg/cmd/destroy"
	"github.com/jenkins-x/jx-preview/pkg/fakescms"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"

	"github.com/jenkins-x/jx-preview/pkg/cmd/create"
	"github.com/stretchr/testify/require"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestPreviewCreate(t *testing.T) {
	containerRegistry := "ghcr.io"
	gitUser := "myuser"
	gitToken := "mytoken"
	owner := "myowner"
	repo := "myrepo"
	ns := "jx"
	prNumber := 5
	branch := "PR-5"
	buildNumber := "2"
	sha := "abcdef1234"
	previewNamespace := ns + "-" + owner + "-" + repo + "-pr-" + strconv.Itoa(prNumber)

	t.Logf("preview in namespace %s", previewNamespace)

	repoLink := "https://" + gitUser + ":" + gitToken + "@fake.com/" + owner + "/" + repo

	previewClient := fake.NewSimpleClientset()

	serviceName := repo
	ingressHost := "hook-jx.1.2.3.4.nip.io"
	previewURL := "http://" + ingressHost

	kubeClient := fakekube.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: previewNamespace,
			},
		},

		// the preview service and ingress resources
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: previewNamespace,
			},
		},

		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: previewNamespace,
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{
					{
						Host: ingressHost,
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{
									{
										Path: "",
										Backend: v1beta1.IngressBackend{
											ServiceName: serviceName,
											ServicePort: intstr.IntOrString{
												IntVal: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	devEnv := jxenv.CreateDefaultDevEnvironment(ns)
	devEnv.Namespace = ns
	devEnv.Spec.Source.URL = "https://github.com/myorg/my-gitops-repo.git"

	jxClient := jxfake.NewSimpleClientset(devEnv)

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
	fakeScmData.CurrentUser.Login = gitUser

	previewName := ""
	for _, testName := range []string{"create", "update"} {
		_, o := create.NewCmdPreviewCreate()
		o.PreviewClient = previewClient
		o.KubeClient = kubeClient
		o.JXClient = jxClient
		o.PullRequestOptions.Options.JXClient = jxClient
		o.Namespace = ns
		o.Branch = branch
		o.GitToken = gitToken
		o.BuildNumber = buildNumber
		o.SourceURL = repoLink + ".git"
		o.Number = prNumber
		o.Dir = "test_data"
		o.DockerRegistry = containerRegistry

		runner := &fakerunner.FakeRunner{
			CommandRunner: func(c *cmdrunner.Command) (string, error) {
				// lets mock running:
				//   helmfile -f preview/helmfile.yaml list --output json
				// after a helm install
				if c.Name == "helmfile" && len(c.Args) > 2 && c.Args[2] == "list" {
					return `[{"name":"preview","namespace":"jx-myowner-myrepo-pr-5","enabled":true,"labels":""}]`, nil
				}
				return "", nil
			},
		}
		o.CommandRunner = runner.Run
		o.ScmClient = scmClient
		o.PreviewURLTimeout = time.Millisecond

		err := o.Run()
		require.NoError(t, err, "failed to run command in test %s", testName)

		runner.ExpectResults(t,
			fakerunner.FakeResult{
				CLI: `helmfile --file test_data/preview/helmfile.yaml sync`,
			},
			fakerunner.FakeResult{
				CLI: `helmfile --file test_data/preview/helmfile.yaml list --output json`,
			},
		)

		previewList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(metav1.ListOptions{})
		require.NoError(t, err, "failed to list previews in namespace %s for test %s", ns, testName)
		require.NotNil(t, previewList, "no preview list returned in namespace %s for test %s", ns, testName)
		require.Len(t, previewList.Items, 1, "previews in namespace %s for test %s", ns, testName)
		preview := previewList.Items[0]

		previewName = preview.Name
		t.Logf("found preview %s in namespace %s for test %s", previewName, ns, testName)

		assert.Equal(t, previewNamespace, preview.Spec.Resources.Namespace, "preview.Spec.Resources.Namespace")
		assert.Equal(t, previewURL, preview.Spec.Resources.URL, "preview.Spec.Resources.URL")

		assert.NotEmpty(t, preview.Spec.DestroyCommand.Args, "preview.Spec.DestroyCommand.Names")
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

	// verify pipeline activity
	actList, err := jxClient.JenkinsV1().PipelineActivities(ns).List(metav1.ListOptions{})
	require.NoError(t, err, "failed to list PipelineActivity in %s", ns)
	require.Len(t, actList.Items, 1, "should have one PipelineActivity")
	activity := actList.Items[0]
	require.Len(t, activity.Spec.Steps, 2, "PipelineActivity.Spec.Steps")
	previewStep := activity.Spec.Steps[1]
	require.NotNil(t, previewStep.Preview, 2, "PipelineActivity.Spec.Steps[1].Preview")
	assert.Equal(t, previewURL, previewStep.Preview.ApplicationURL, "PipelineActivity.Spec.Steps[1].ApplicationURL")
	assert.Equal(t, prLink, previewStep.Preview.PullRequestURL, "PipelineActivity.Spec.Steps[1].PullRequestURL")
	t.Logf("found PipelineActivity %s with app URL: %s and PR URL: %s", activity.Name, previewStep.Preview.ApplicationURL, previewStep.Preview.PullRequestURL)

	// now lets test deleting the preview
	_, do := destroy.NewCmdPreviewDestroy()
	do.PreviewClient = previewClient
	do.Namespace = ns
	do.Names = []string{previewName}

	runner := &fakerunner.FakeRunner{}
	do.CommandRunner = runner.Run

	do.KubeClient = kubeClient

	err = do.Run()
	require.NoError(t, err, "failed to delete preview %s", previewName)

	require.Len(t, runner.OrderedCommands, 2, "should have 2 commands")
	assert.Equal(t, `helmfile --file test_data/preview/helmfile.yaml destroy`, runner.OrderedCommands[1].CLI(), "second command")

	// now lets check we removed the preview namespace
	namespaceList, err := do.KubeClient.CoreV1().Namespaces().List(metav1.ListOptions{})
	require.NoError(t, err, "failed to list namespaces")
	require.Len(t, namespaceList.Items, 0, "should not have any Namespaces")
}

func TestPreviewCreateHelmfileDiscovery(t *testing.T) {
	appsDirRelPath := filepath.Join("charts", "myapp")
	testCases := []struct {
		name    string
		relPath string
	}{
		{
			name: "rootDir",
		},
		{
			name:    "appsDir",
			relPath: appsDirRelPath,
		},
	}

	runner := &fakerunner.FakeRunner{}
	scmClient, fakeData := fakescm.NewDefault()

	owner := "myowner"
	repo := "myrepo"
	sourceURL := "https://github.com/" + owner + "/" + repo

	fakescms.CreatePullRequest(fakeData, owner, repo, 1)

	for _, tc := range testCases {
		tmpDir, err := ioutil.TempDir("", "")
		require.NoError(t, err, "could not create temp dir")

		appsDir := filepath.Join(tmpDir, appsDirRelPath)
		err = os.MkdirAll(appsDir, files.DefaultDirWritePermissions)
		require.NoError(t, err, "could not create apps chart dir %s", appsDir)

		_, o := create.NewCmdPreviewCreate()
		o.CommandRunner = runner.Run
		o.Dir = tmpDir

		// values for PR discovery
		o.Number = 1
		o.ScmClient = scmClient
		o.Branch = "PR-1"
		o.SourceURL = sourceURL
		o.PullRequestBranch = "master"

		if tc.relPath != "" {
			o.Dir = filepath.Join(tmpDir, tc.relPath)
		}

		err = o.DiscoverPreviewHelmfile()
		require.NoError(t, err, "failed to run for test %s", tc.name)

		assert.Equal(t, filepath.Join(tmpDir, "preview", "helmfile.yaml"), o.PreviewHelmfile, "for test %s", tc.name)
		//require.FileExists(t, filepath.Join(tmpDir, "charts", "preview", "helmfile.yaml"), "should have created helmfile.yaml")
	}

	for _, c := range runner.OrderedCommands {
		t.Logf("fake command: %s\n", c.CLI())
	}
}

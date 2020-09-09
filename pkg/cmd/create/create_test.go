package create_test

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jenkins-x/go-scm/scm"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	jxfake "github.com/jenkins-x/jx-api/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner/fakerunner"
	"github.com/jenkins-x/jx-helpers/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-preview/pkg/cmd/destroy"
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

	helmFile := filepath.Join("test_data", "charts", "preview", "helmfile.yaml")
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
		o.Namespace = ns
		o.Branch = branch
		o.GitToken = gitToken
		o.BuildNumber = buildNumber
		o.SourceURL = repoLink + ".git"
		o.Number = prNumber
		o.PreviewHelmfile = helmFile
		o.DockerRegistry = containerRegistry

		runner := &fakerunner.FakeRunner{
			CommandRunner: func(c *cmdrunner.Command) (string, error) {
				// lets mock running:
				//   helmfile -f charts/preview/helmfile.yaml list --output json
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
				CLI: `helmfile --file test_data/charts/preview/helmfile.yaml sync`,
			},
			fakerunner.FakeResult{
				CLI: `helmfile --file test_data/charts/preview/helmfile.yaml list --output json`,
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

		assert.Equal(t, previewURL, preview.Status.ApplicationURL, "preview.Status.ApplicationURL")

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
	do.Name = previewName

	runner := &fakerunner.FakeRunner{}
	do.CommandRunner = runner.Run

	do.KubeClient = kubeClient

	err = do.Run()
	require.NoError(t, err, "failed to delete preview %s", previewName)

	require.Len(t, runner.OrderedCommands, 2, "should have 2 commands")
	assert.Equal(t, `helmfile --file test_data/charts/preview/helmfile.yaml destroy`, runner.OrderedCommands[1].CLI(), "second command")

	// now lets check we removed the preview namespace
	namespaceList, err := do.KubeClient.CoreV1().Namespaces().List(metav1.ListOptions{})
	require.NoError(t, err, "failed to list namespaces")
	require.Len(t, namespaceList.Items, 0, "should not have any Namespaces")
}

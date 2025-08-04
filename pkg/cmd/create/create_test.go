package create_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"

	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x-plugins/jx-preview/pkg/cmd/destroy"
	"github.com/jenkins-x-plugins/jx-preview/pkg/fakescms"
	"github.com/jenkins-x/go-scm/scm"
	fakescm "github.com/jenkins-x/go-scm/scm/driver/fake"
	jxfake "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner/fakerunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	nv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"
	kservefake "knative.dev/serving/pkg/client/clientset/versioned/fake"

	"github.com/jenkins-x-plugins/jx-preview/pkg/cmd/create"
	"github.com/stretchr/testify/require"
)

func TestPreviewCreate(t *testing.T) {
	AssertPreview(t, "", false, "Running", 1, nil)
}

func TestPreviewCreateWithCustomService(t *testing.T) {
	AssertPreview(t, "custom-service", false, "Running", 0, nil)
}

func TestPreviewCreateWithSelector(t *testing.T) {
	AssertPreview(t, "", false, "Running", 1, []string{"app=my-app"})
}

func TestPreviewCreateWithMultipleSelectors(t *testing.T) {
	AssertPreview(t, "", false, "Running", 1, []string{"tier=frontend,tier!=proxy", "tier=backend"})
}

func TestHelmfileSyncFailurePostsPodLogs(t *testing.T) {
	AssertPreview(t, "", true, "Pending", 8, nil)
	AssertPreview(t, "", true, "Failed", 0, nil)
}

func AssertPreview(t *testing.T, customService string, failSync bool, podState corev1.PodPhase, numberOfRestarts int32, selectors []string) {
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
	defaultService := "jx-service"

	t.Logf("preview in namespace %s", previewNamespace)

	repoLink := "https://" + gitUser + ":" + gitToken + "@fake.com/" + owner + "/" + repo

	previewClient := fake.NewSimpleClientset()

	serviceName := customService
	if serviceName == "" {
		serviceName = defaultService
	}

	ingressHost := serviceName + ".1.2.3.4.nip.io"
	previewPath := "cheese"
	previewURL := "http://" + ingressHost
	if previewPath != "" {
		previewURL = stringhelpers.UrlJoin(previewURL, previewPath)
	}

	kubeClient := SetupKubeClient(serviceName, previewNamespace, ingressHost, podState, numberOfRestarts)

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

	ctx := context.Background()
	previewName := ""
	tmpDir := ""
	for _, testName := range []string{"create", "update"} {
		_, o := create.NewCmdPreviewCreate()
		o.GitUser = "fakeuser"
		o.GitToken = "faketoken"
		o.NoWatchNamespace = true
		o.PreviewClient = previewClient
		o.KubeClient = kubeClient
		o.JXClient = jxClient
		o.KServeClient = kservefake.NewSimpleClientset()
		o.PullRequestOptions.Options.JXClient = jxClient
		o.Namespace = ns
		o.Branch = branch
		o.GitToken = gitToken
		o.BuildNumber = buildNumber
		o.SourceURL = repoLink + ".git"
		o.Number = prNumber
		o.PreviewURLPath = previewPath
		o.Selectors = selectors
		if customService != "" {
			o.PreviewService = customService
		}

		var err error
		tmpDir, err = os.MkdirTemp("", "")
		require.NoError(t, err, "failed to create temp dir")

		err = files.CopyDirOverwrite("test_data", tmpDir)
		require.NoError(t, err, "failed to copy test_data to %s", tmpDir)
		o.Dir = tmpDir
		o.DockerRegistry = containerRegistry

		runner := &fakerunner.FakeRunner{
			CommandRunner: func(c *cmdrunner.Command) (string, error) {
				// lets mock running:
				//   helmfile -f preview/helmfile.yaml.gotmpl list --output json
				// after a helm install
				if c.Name == "helmfile" && len(c.Args) > 2 && c.Args[2] == "list" {
					s := `[{"name":"preview","namespace":"jx-myowner-myrepo-pr-5","enabled":true,"labels":""}]`
					if c.Out != nil {
						_, _ = c.Out.Write([]byte(s))
						return "", nil
					}
					return s, nil
				}
				// helmfile sync
				if c.Name == "helmfile" && c.Args[2] == "sync" && failSync {
					return " \tCOMBINED OUTPUT:\n\t\t  Error: UPGRADE FAILED: timed out waiting for the condition", errors.New(" \tCOMBINED OUTPUT:\n\t\t  Error: UPGRADE FAILED: timed out waiting for the condition'")
				}
				// helmfile sync
				if c.Name == "helmfile" && c.Args[2] == "sync" {
					if failSync {
						return " \tCOMBINED OUTPUT:\n\t\t  Error: UPGRADE FAILED: timed out waiting for the condition", errors.New(" \tCOMBINED OUTPUT:\n\t\t  Error: UPGRADE FAILED: timed out waiting for the condition'")
					}
					if len(selectors) > 0 {
						expectedSelector := "--selector=" + selectors[0]
						assert.Contains(t, c.Args, expectedSelector, "helmfile sync command should contain the selector")
					}
				}
				// git clone
				if c.Name == "git" && c.Args[0] == "clone" {
					err = os.MkdirAll(filepath.Join(c.Args[2], "helmfiles", "jx"), 0755)
					if err != nil {
						return "", err
					}
					err = os.WriteFile(filepath.Join(c.Args[2], "helmfiles", "jx", "jx-values.yaml"), []byte(""), 0600)
					if err != nil {
						return "", err
					}
				}
				return "", nil
			},
		}
		o.CommandRunner = runner.Run
		o.ScmClient = scmClient
		o.PreviewURLTimeout = time.Millisecond
		o.Version = "0.0.0-SNAPSHOT-PR-1"
		err = o.Run()
		if failSync {
			require.Error(t, err, "should have failed to create/update the preview environment")
			require.Contains(t, err.Error(), "timed out waiting for the condition", "should have timed out via the helmfile sync")
			require.Contains(t, err.Error(), "fake logs", "should have returned the fake logs")
			// If the sync fails the pipeline wont be updated so we need to return
			return
		}

		require.NoError(t, err, "failed to run command in test %s", testName)

		previewList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(ctx, metav1.ListOptions{})
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

	// If the sync fails the pipeline wont be updated
	if failSync {
		return
	}

	// verify pipeline activity
	actList, err := jxClient.JenkinsV1().PipelineActivities(ns).List(ctx, metav1.ListOptions{})
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
	do.GitUser = "fakeuser"
	do.GitToken = "faketoken"
	do.PreviewClient = previewClient
	do.Namespace = ns
	do.Names = []string{previewName}

	runner := &fakerunner.FakeRunner{
		CommandRunner: func(c *cmdrunner.Command) (string, error) {
			if c.Name == "git" && len(c.Args) > 0 && c.Args[0] == "clone" {
				dir := c.Args[len(c.Args)-1]
				repoURL := c.Args[len(c.Args)-2]
				if strings.HasSuffix(repoURL, "repo.git") {
					err = os.MkdirAll(filepath.Join(c.Args[2], "helmfiles", "jx"), 0755)
					if err != nil {
						return "", err
					}
					err = os.WriteFile(filepath.Join(c.Args[2], "helmfiles", "jx", "jx-values.yaml"), []byte(""), 0600)
					if err != nil {
						return "", err
					}
				} else {
					// lets copy the sample project
					srcDir := filepath.Join("test_data", "sample_project")
					err := files.CopyDirOverwrite(srcDir, dir)
					if err != nil {
						return "", fmt.Errorf("failed to copy files from %s to %s: %w", srcDir, dir, err)
					}
				}
				return "", nil
			}
			t.Logf("faking command %s in dir %s\n", c.CLI(), c.Dir)
			return "", nil
		},
	}
	do.CommandRunner = runner.Run

	do.KubeClient = kubeClient
	do.JXClient = jxClient
	do.ScmClient = scmClient
	do.Branch = "master"

	err = do.Run()
	require.NoError(t, err, "failed to delete preview %s", previewName)

	require.Len(t, runner.OrderedCommands, 3, "should have 2 commands")
	assert.Equal(t, "helmfile --file "+tmpDir+"/preview/helmfile.yaml.gotmpl destroy", runner.OrderedCommands[2].CLI(), "second command")

	// now lets check we removed the preview namespace
	namespaceList, err := do.KubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "failed to list namespaces")
	require.Len(t, namespaceList.Items, 0, "should not have any Namespaces")
}

func SetupKubeClient(serviceName, previewNamespace, ingressHost string, podState corev1.PodPhase, restarts int32) kubernetes.Interface {
	return fakekube.NewSimpleClientset(
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

		// Create a failed pod, only looked at if the helmfile failed
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "failed-pod",
				Namespace: previewNamespace,
			},
			Status: corev1.PodStatus{
				Phase: podState,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "failed-container",
						RestartCount: restarts,
					},
				},
			},
		},

		&nv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: previewNamespace,
			},
			Spec: nv1.IngressSpec{
				Rules: []nv1.IngressRule{
					{
						Host: ingressHost,
						IngressRuleValue: nv1.IngressRuleValue{
							HTTP: &nv1.HTTPIngressRuleValue{
								Paths: []nv1.HTTPIngressPath{
									{
										Path: "",
										Backend: nv1.IngressBackend{
											Service: &nv1.IngressServiceBackend{
												Name: serviceName,
												Port: nv1.ServiceBackendPort{
													Number: 80,
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
		},
	)
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
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err, "could not create temp dir")

		appsDir := filepath.Join(tmpDir, appsDirRelPath)
		err = os.MkdirAll(appsDir, files.DefaultDirWritePermissions)
		require.NoError(t, err, "could not create apps chart dir %s", appsDir)

		_, o := create.NewCmdPreviewCreate()
		o.GitUser = "fakeuser"
		o.GitToken = "faketoken"
		o.NoWatchNamespace = true
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
		require.Error(t, err, "run should fail since the next run should start automatically on push %s", tc.name)

		assert.Equal(t, filepath.Join(tmpDir, "preview", "helmfile.yaml.gotmpl"), o.PreviewHelmfile, "for test %s", tc.name)
	}

	for _, c := range runner.OrderedCommands {
		t.Logf("fake command: %s\n", c.CLI())
	}
}

func TestHelfileV1Compatbility(t *testing.T) {
	runner := &fakerunner.FakeRunner{}
	scmClient, fakeData := fakescm.NewDefault()

	owner := "myowner"
	repo := "myrepo"
	sourceURL := "https://github.com/" + owner + "/" + repo

	fakescms.CreatePullRequest(fakeData, owner, repo, 1)

	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err, "could not create temp dir")
	err = files.CopyDirOverwrite("test_data", tmpDir)
	require.NoError(t, err, "could not copy test data")
	err = files.DeleteFile(filepath.Join(tmpDir, "preview", "helmfile.yaml.gotmpl"))
	require.NoError(t, err, "could adjust test data")
	err = files.CopyDirOverwrite(filepath.Join(tmpDir, "oldPreview"), filepath.Join(tmpDir, "preview"))
	require.NoError(t, err, "could adjust test data")

	_, o := create.NewCmdPreviewCreate()
	o.GitUser = "fakeuser"
	o.GitToken = "faketoken"
	o.NoWatchNamespace = true
	o.CommandRunner = runner.Run
	o.Dir = tmpDir

	// values for PR discovery
	o.Number = 1
	o.ScmClient = scmClient
	o.Branch = "PR-1"
	o.SourceURL = sourceURL
	o.PullRequestBranch = "master"

	err = o.DiscoverPreviewHelmfile()
	require.Error(t, err, "run should fail since the next run should start automatically on push")

	previewHelmfile := filepath.Join(tmpDir, "preview", "helmfile.yaml.gotmpl")
	assert.Equal(t, previewHelmfile, o.PreviewHelmfile)
	assert.FileExists(t, previewHelmfile)
}

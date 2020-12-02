package create

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"context"
	"fmt"

	"github.com/cenkalti/backoff"
	"github.com/jenkins-x/go-scm/scm"
	jxc "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-gitops/pkg/cmd/git/get"
	"github.com/jenkins-x/jx-gitops/pkg/cmd/pr/push"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/cli"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/activities"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/services"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-preview/pkg/helmfiles"
	"github.com/jenkins-x/jx-preview/pkg/kserving"
	"github.com/jenkins-x/jx-preview/pkg/previews"
	"github.com/jenkins-x/jx-preview/pkg/rootcmd"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kserve "knative.dev/serving/pkg/client/clientset/versioned"
)

var (
	cmdLong = templates.LongDesc(`
		Creates a preview
`)

	cmdExample = templates.Examples(`
		# creates a new preview environemnt
		%s create
	`)

	info = termcolor.ColorInfo
)

// Options the CLI options for
type Options struct {
	scmhelpers.PullRequestOptions

	PreviewHelmfile  string
	PreviewNamespace string
	Namespace        string
	DockerRegistry   string
	BuildNumber      string
	Version          string
	// PullRequestBranch used for testing to fake out the pull request branch name
	PullRequestBranch string
	PreviewURLTimeout time.Duration
	NoComment         bool
	Debug             bool
	PreviewClient     versioned.Interface
	KubeClient        kubernetes.Interface
	JXClient          jxc.Interface
	KServeClient      kserve.Interface
	CommandRunner     cmdrunner.CommandRunner
}

type envVar struct {
	Name         string
	DefaultValue func() string
}

// NewCmdPreviewCreate creates a command object for the command
func NewCmdPreviewCreate() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Creates a preview",
		Long:    cmdLong,
		Example: fmt.Sprintf(cmdExample, rootcmd.BinaryName),
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.PreviewHelmfile, "file", "f", "", "Preview helmfile.yaml path to use. If not specified it is discovered in preview/helmfile.yaml and created from a template if needed")
	cmd.Flags().StringVarP(&o.Repository, "app", "", "", "Name of the app or repository")
	cmd.Flags().DurationVarP(&o.PreviewURLTimeout, "preview-url-timeout", "", time.Minute+5, "Time to wait for the preview URL to be available")
	cmd.Flags().BoolVarP(&o.NoComment, "no-comment", "", false, "Disables commenting on the Pull Request after preview is created")
	cmd.Flags().BoolVarP(&o.Debug, "debug", "", false, "Enables debug logging in helmfile")

	o.PullRequestOptions.AddFlags(cmd)
	return cmd, o
}

// Run implements a helmfile based preview environment
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	pr, err := o.DiscoverPullRequest()
	if err != nil {
		return errors.Wrapf(err, "failed to discover pull request")
	}

	log.Logger().Infof("found PullRequest %s", pr.Link)

	envVars, err := o.createHelmfileEnvVars()
	if err != nil {
		return errors.Wrapf(err, "failed to create env vars")
	}

	destroyCmd := o.createDestroyCommand(envVars)

	// lets get the git clone URL with user/password so we can clone it again in the destroy command/CronJob
	ctx := context.Background()
	username := ""
	user, _, err := o.ScmClient.Users.Find(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to find the current SCM user")
	}
	if user != nil {
		username = user.Login
	}
	gitURL, err := stringhelpers.URLSetUserPassword(pr.Repository().Link, username, o.GitToken)
	if err != nil {
		return errors.Wrapf(err, "failed to modify the git URL")
	}

	err = previews.CreateJXValuesFile(o.Options, o.JXClient, o.Namespace, o.PreviewHelmfile, o.PreviewNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create the jx-values.yaml file")
	}

	err = o.createImagePullSecretsFile()
	if err != nil {
		return errors.Wrapf(err, "failed to create the imagePullSecrets.yaml file")
	}

	preview, _, err := previews.GetOrCreatePreview(o.PreviewClient, o.Namespace, pr, destroyCmd, gitURL, o.PreviewNamespace, o.PreviewHelmfile)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert the Preview resource in namespace %s", o.Namespace)
	}
	if preview == nil {
		return errors.Errorf("no upserted Preview resource in namespace %s", o.Namespace)
	}
	log.Logger().Infof("upserted preview %s", preview.Name)

	err = o.helmfileSyncPreview(envVars)
	if err != nil {
		return errors.Wrapf(err, "failed to helmfile sync")
	}

	url, err := o.findPreviewURL(envVars)
	if err != nil {
		log.Logger().Warnf("failed to detect the preview URL %+v", err)
	}

	toAuthor(&preview.Spec.PullRequest.User, &pr.Author)

	if url != "" {
		log.Logger().Infof("preview %s is now running at %s", info(preview.Name), info(url))

		// lets modify the preview
		preview.Spec.Resources.Name = o.Repository
		preview.Spec.Resources.URL = url
		preview, err = o.PreviewClient.PreviewV1alpha1().Previews(o.Namespace).Update(ctx, preview, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update preview %s", preview.Name)
		}
		log.Logger().Infof("updated preview %s with URL %s", preview.Name, url)
	} else {
		log.Logger().Infof("could not detect a preview URL")
	}

	err = o.updatePipelineActivity(url, preview.Spec.PullRequest.URL)
	if err != nil {
		log.Logger().Warnf("failed to update the PipelineActivity - are you using Jenkins X? %s", err.Error())
	}

	if o.NoComment {
		return nil
	}

	return o.commentOnPullRequest(preview, url)
}

func toAuthor(to *v1alpha1.UserSpec, from *scm.User) {
	if from == nil {
		return
	}
	if from.Login != "" {
		to.Username = from.Login
	}
	if from.Name != "" {
		to.Name = from.Name
	}
	if from.Avatar != "" {
		to.ImageURL = from.Avatar
	}
}

// Validate validates the inputs are valid
func (o *Options) Validate() error {
	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.DefaultCommandRunner
	}
	err := o.PullRequestOptions.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate repository options")
	}

	err = o.DiscoverPreviewHelmfile()
	if err != nil {
		return errors.Wrapf(err, "failed to discover the preview helmfile")
	}

	o.PreviewClient, o.Namespace, err = previews.LazyCreatePreviewClientAndNamespace(o.PreviewClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create Preview client")
	}

	o.KubeClient, err = kube.LazyCreateKubeClient(o.KubeClient)
	if err != nil {
		return errors.Wrapf(err, "failed to create kube client")
	}
	o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
	if err != nil {
		return errors.Wrapf(err, "failed to create jx client")
	}

	o.KServeClient, err = kserving.LazyCreateKServeClient(o.KServeClient)
	if err != nil {
		return errors.Wrapf(err, "failed to create jx client")
	}

	if o.PreviewNamespace == "" {
		o.PreviewNamespace, err = o.createPreviewNamespace()
	}
	return nil
}

func (o *Options) createDestroyCommand(envVars map[string]string) v1alpha1.Command {
	args := []string{"--file", o.PreviewHelmfile}
	if o.Debug {
		args = append(args, "--debug")
	}
	args = append(args, "destroy")

	var env []v1alpha1.EnvVar
	if envVars != nil {
		for k, v := range envVars {
			env = append(env, v1alpha1.EnvVar{
				Name:  k,
				Value: v,
			})
		}
	}
	return v1alpha1.Command{
		Command: "helmfile",
		Args:    args,
		Env:     env,
	}
}

func (o *Options) helmfileSyncPreview(envVars map[string]string) error {
	log.Logger().Infof("passing env vars into helmfile: %#v", envVars)

	args := []string{"--file", o.PreviewHelmfile}
	if o.Debug {
		args = append(args, "--debug")
	}

	// first lets always make sure we have the latest helm repo updates
	c := &cmdrunner.Command{
		Name: "helmfile",
		Args: append(args, "repos"),
		Env:  envVars,
	}
	_, err := o.CommandRunner(c)
	if err != nil {
		return errors.Wrapf(err, "failed to run helmfile repos")
	}

	// now install the charts using sync
	c = &cmdrunner.Command{
		Name: "helmfile",
		Args: append(args, "sync"),
		Env:  envVars,
	}
	_, err = o.CommandRunner(c)
	if err != nil {
		return errors.Wrapf(err, "failed to run helmfile sync")
	}
	return nil
}

func (o *Options) createHelmfileEnvVars() (map[string]string, error) {
	env := map[string]string{}
	mandatoryEnvVars := []envVar{
		{
			Name: "APP_NAME",
			DefaultValue: func() string {
				return o.Repository
			},
		},
		{
			Name: "DOCKER_REGISTRY",
			DefaultValue: func() string {
				return o.DockerRegistry
			},
		},
		{
			Name: "DOCKER_REGISTRY_ORG",
			DefaultValue: func() string {
				return o.Options.Owner
			},
		},
		{
			Name: "PREVIEW_NAMESPACE",
			DefaultValue: func() string {
				return o.PreviewNamespace
			},
		},
		{
			Name: "VERSION",
			DefaultValue: func() string {
				return o.Version
			},
		},
	}

	for _, e := range mandatoryEnvVars {
		name := e.Name
		value := os.Getenv(name)
		if value == "" && e.DefaultValue != nil {
			value = e.DefaultValue()
		}
		if value == "" {
			return env, errors.Errorf("missing $%s environment variable", name)
		}
		env[name] = value
	}
	return env, nil

}

func (o *Options) createPreviewNamespace() (string, error) {
	prefix := o.Namespace + "-"
	prName := o.Owner + "-" + o.Repository + "-pr-" + strconv.Itoa(o.Number)

	prNamespace := prefix + prName
	if len(prNamespace) > 63 {
		max := 62 - len(prefix)
		size := len(prName)

		prNamespace = prefix + prName[size-max:]
		log.Logger().Warnf("Due the name of the organisation and repository being too long (%s) we are going to trim it to make the preview namespace: %s", prName, prNamespace)
	}
	if len(prNamespace) > 63 {
		return "", fmt.Errorf("preview namespace %s is too long. Must be no more than 63 character", prNamespace)
	}
	prNamespace = naming.ToValidName(prNamespace)
	return prNamespace, nil
}

// findPreviewURL finds the preview URL
func (o *Options) findPreviewURL(envVars map[string]string) (string, error) {

	releases, err := helmfiles.ListReleases(o.CommandRunner, o.PreviewHelmfile, envVars)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read helmfile releases")
	}

	// lets try find the release name
	if len(releases) == 0 {
		return "", errors.Errorf("helmfile %s has no releases", o.PreviewHelmfile)
	}

	// lets assume first release is the preview
	release := releases[0]
	releaseName := release.Name
	if releaseName == "" {
		log.Logger().Warnf("the helmfile %s has no name for the first release", o.PreviewHelmfile)
		releaseName = "preview"
	}
	releaseNamespace := release.Namespace
	if releaseNamespace == "" {
		releaseNamespace = o.PreviewNamespace
	}

	log.Logger().Infof("found helm preview release %s in namespace %s", info(releaseName), info(releaseNamespace))

	application := o.Repository
	app := naming.ToValidName(application)

	appNames := []string{app, releaseName, o.Namespace + "-preview", releaseName + "-" + app}

	ctx := context.Background()

	url := ""
	fn := func() error {
		for _, n := range appNames {
			url, _ = services.FindServiceURL(o.KubeClient, releaseNamespace, n)
			if url == "" {
				var err error
				url, _, err = kserving.FindServiceURL(ctx, o.KServeClient, o.KubeClient, releaseNamespace, n)
				if err != nil {
					return errors.Wrapf(err, "failed to ")
				}
			}
			if url != "" {
				return nil
			}
		}
		return errors.Errorf("not found")
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = o.PreviewURLTimeout
	bo.Reset()
	err = backoff.Retry(fn, bo)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find preview URL for app names %#v in timeout %v", appNames, o.PreviewURLTimeout)
	}
	return url, nil
}

func (o *Options) updatePipelineActivity(applicationURL, pullRequestURL string) error {
	if applicationURL == "" {
		return nil
	}
	if o.BuildNumber == "" {
		o.BuildNumber = os.Getenv("BUILD_NUMBER")
		if o.BuildNumber == "" {
			o.BuildNumber = os.Getenv("BUILD_ID")
		}
	}
	pipeline := fmt.Sprintf("%s/%s/%s", o.Owner, o.Repository, o.Branch)

	ctx := context.Background()
	build := o.BuildNumber
	if pipeline != "" && build != "" {
		ns := o.Namespace
		name := naming.ToValidName(pipeline + "-" + build)

		jxClient := o.JXClient

		// lets see if we can update the pipeline
		acts := jxClient.CoreV4beta1().PipelineActivities(ns)
		key := &activities.PromoteStepActivityKey{
			PipelineActivityKey: activities.PipelineActivityKey{
				Name:     name,
				Pipeline: pipeline,
				Build:    build,
				GitInfo: &giturl.GitRepository{
					Name:         o.Repository,
					Organisation: o.Owner,
				},
			},
		}
		a, _, p, _, err := key.GetOrCreatePreview(jxClient, ns)
		if err == nil && a != nil && p != nil {
			updated := false
			if p.ApplicationURL == "" {
				p.ApplicationURL = applicationURL
				updated = true
			}
			if p.PullRequestURL == "" && pullRequestURL != "" {
				p.PullRequestURL = pullRequestURL
				updated = true
			}
			if updated {
				a, err = acts.Update(ctx, a, metav1.UpdateOptions{})
				if err != nil {
					log.Logger().Warnf("Failed to update PipelineActivity %s: %s", name, err.Error())
				} else {
					log.Logger().Infof("Updated PipelineActivity %s which has status %s", name, string(a.Spec.Status))
				}
			}
		}
	} else {
		log.Logger().Warnf("No pipeline and build number available on $JOB_NAME and $BUILD_NUMBER so cannot update PipelineActivities with the preview URLs")
	}
	return nil
}

func (o *Options) commentOnPullRequest(preview *v1alpha1.Preview, url string) error {
	comment := fmt.Sprintf(":star: PR built and available in a preview **%s**", preview.Name)
	if url != "" {
		comment += fmt.Sprintf(" [here](%s) ", url)
	}

	ctx := context.Background()
	commentInput := &scm.CommentInput{
		Body: comment,
	}
	_, _, err := o.ScmClient.PullRequests.CreateComment(ctx, o.FullRepositoryName, o.Number, commentInput)
	prName := "#" + strconv.Itoa(o.Number)
	if err != nil {
		return errors.Wrapf(err, "failed to comment on pull request %s on repository %s", prName, o.FullRepositoryName)
	}
	log.Logger().Infof("commented on pull request %s on repository %s", info(prName), info(o.FullRepositoryName))
	return nil
}

func (o *Options) createImagePullSecretsFile() error {
	_, getOpts := get.NewCmdGitGet()

	getOpts.Options = o.Options
	getOpts.Env = "dev"
	getOpts.Path = "imagePullSecrets.yaml"
	getOpts.JXClient = o.JXClient
	getOpts.Namespace = o.Namespace
	getOpts.To = filepath.Join(filepath.Dir(o.PreviewHelmfile), "imagePullSecrets.yaml")
	err := getOpts.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to get the file %s from Environment %s", getOpts.Path, getOpts.Env)
	}
	return nil
}

// DiscoverPreviewHelmfile if there is no helmfile configured
// lets find the charts folder and default the preview helmfile to that
// then generate a helmfile.yaml if its missing
func (o *Options) DiscoverPreviewHelmfile() error {
	if o.PreviewHelmfile == "" {
		chartsDir := filepath.Join(o.Dir, "charts")
		exists, err := files.DirExists(chartsDir)
		if err != nil {
			return errors.Wrapf(err, "failed to check if dir %s exists", chartsDir)
		}
		if !exists {
			chartsDir = filepath.Join(o.Dir, "..")
			exists, err = files.DirExists(chartsDir)
			if err != nil {
				return errors.Wrapf(err, "failed to check if dir %s exists", chartsDir)
			}
			if !exists {
				return errors.Wrapf(err, "could not detect the helm charts folder in dir %s", o.Dir)
			}
		}

		o.PreviewHelmfile = filepath.Join(chartsDir, "..", "preview", "helmfile.yaml")
	}

	exists, err := files.FileExists(o.PreviewHelmfile)
	if err != nil {
		return errors.Wrapf(err, "failed to check for file %s", o.PreviewHelmfile)
	}
	if exists {
		return nil
	}

	// lets make the preview dir
	previewDir := filepath.Dir(o.PreviewHelmfile)
	parentDir := filepath.Dir(previewDir)
	relDir, err := filepath.Rel(o.Dir, parentDir)
	if err != nil {
		return errors.Wrapf(err, "failed to find preview dir in %s of %s", o.Dir, parentDir)
	}
	err = os.MkdirAll(parentDir, files.DefaultDirWritePermissions)
	if err != nil {
		return errors.Wrapf(err, "failed to make preview dir %s", parentDir)
	}

	// now lets grab the template preview
	c := &cmdrunner.Command{
		Dir:  o.Dir,
		Name: "kpt",
		Args: []string{"pkg", "get", "https://github.com/jenkins-x/jx3-gitops-template.git/charts/preview", relDir},
	}
	_, err = o.CommandRunner(c)
	if err != nil {
		return errors.Wrapf(err, "failed to get the preview helmfile: %s via kpt", o.PreviewHelmfile)
	}

	// lets add the files
	if o.GitClient == nil {
		o.GitClient = cli.NewCLIClient("", o.CommandRunner)
	}
	_, err = o.GitClient.Command(o.Dir, "add", "*")
	if err != nil {
		return errors.Wrapf(err, "failed to add the preview helmfile files to git")
	}
	_, err = o.GitClient.Command(o.Dir, "commit", "-a", "-m", "fix: add preview helmfile")
	if err != nil {
		return errors.Wrapf(err, "failed to commit the preview helmfile files to git")
	}

	// lets push the changes to git
	_, po := push.NewCmdPullRequestPush()
	po.CommandRunner = o.CommandRunner
	po.ScmClient = o.ScmClient
	po.SourceURL = o.SourceURL
	po.Number = o.Number
	po.Branch = o.PullRequestBranch

	err = po.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to push the changes to git")
	}
	return nil
}

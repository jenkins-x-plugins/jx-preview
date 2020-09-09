package create

import (
	"os"
	"strconv"
	"time"

	"context"
	"fmt"

	"github.com/cenkalti/backoff"
	"github.com/jenkins-x/go-scm/scm"
	jxc "github.com/jenkins-x/jx-api/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/kube/activities"
	"github.com/jenkins-x/jx-helpers/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/pkg/kube/services"
	"github.com/jenkins-x/jx-helpers/pkg/options"
	"github.com/jenkins-x/jx-helpers/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-preview/pkg/helmfiles"
	"github.com/jenkins-x/jx-preview/pkg/previews"
	"github.com/jenkins-x/jx-preview/pkg/rootcmd"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
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
	scmhelpers.Options

	PreviewHelmfile   string
	PreviewNamespace  string
	Number            int
	Namespace         string
	DockerRegistry    string
	BuildNumber       string
	PreviewURLTimeout time.Duration
	NoComment         bool
	Debug             bool
	PreviewClient     versioned.Interface
	KubeClient        kubernetes.Interface
	JXClient          jxc.Interface
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
	cmd.Flags().IntVarP(&o.Number, "pr", "", 0, "the Pull Request number. If not specified we will use $BRANCH_NAME")
	cmd.Flags().DurationVarP(&o.PreviewURLTimeout, "preview-url-timeout", "", time.Minute+5, "the time to wait for the preview URL to be available")
	cmd.Flags().BoolVarP(&o.NoComment, "no-comment", "", false, "Disables commenting on the Pull Request after preview is created")

	o.Options.AddFlags(cmd)
	return cmd, o
}

// Run implements a helmfile based preview environment
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	pr, err := o.discoverPullRequest()
	if err != nil {
		return errors.Wrapf(err, "failed to discover pull request")
	}

	log.Logger().Infof("found PullRequest %s", pr.Link)

	envVars, err := o.createHelmfileEnvVars()
	if err != nil {
		return errors.Wrapf(err, "failed to create env vars")
	}

	destroyCmd := o.createDestroyCommand(envVars)

	preview, _, err := previews.GetOrCreatePreview(o.PreviewClient, o.Namespace, pr, destroyCmd, o.PreviewNamespace, o.PreviewHelmfile)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert the Preview resource in namespace %s", o.Namespace)
	}
	log.Logger().Infof("upserted preview %s", preview.Name)

	err = o.helmfileSyncPreview(envVars)
	if err != nil {
		return errors.Wrapf(err, "failed to helmfile sync")
	}

	url, err := o.findPreviewURL(envVars)
	if err != nil {
		return errors.Wrapf(err, "failed to detect the preview URL")
	}

	err = o.updatePipelineActivity(url, preview.Spec.PullRequest.URL)
	if err != nil {
		return errors.Wrapf(err, "failed to update PipelineActivity")
	}

	if url != "" {
		log.Logger().Infof("the preview %s is now running at %s", info(preview.Name), info(url))

		// lets modify the preview
		preview.Status.ApplicationName = o.Repository
		preview.Status.ApplicationURL = url
		preview, err = o.PreviewClient.PreviewV1alpha1().Previews(o.Namespace).Update(preview)
		if err != nil {
			return errors.Wrapf(err, "failed to update preview %s", preview.Name)
		}
	}
	if o.NoComment {
		return nil
	}

	return o.commentOnPullRequest(preview, url)
}

// Validate validates the inputs are valid
func (o *Options) Validate() error {
	err := o.Options.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate repository options")
	}

	if o.PreviewHelmfile == "" {
		return options.MissingOption("helmfile")
	}
	if o.Number <= 0 {
		prName := os.Getenv("PULL_NUMBER")
		if prName != "" {
			o.Number, err = strconv.Atoi(prName)
			if err != nil {
				log.Logger().Warnf(
					"Unable to convert PR " + prName + " to a number")
			}
		}
		if o.Number <= 0 {
			return options.MissingOption("pr")
		}
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

	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.DefaultCommandRunner
	}
	if o.PreviewNamespace == "" {
		o.PreviewNamespace, err = o.createPreviewNamespace()
	}
	return nil
}

func (o *Options) discoverPullRequest() (*scm.PullRequest, error) {
	ctx := context.Background()
	pr, _, err := o.ScmClient.PullRequests.Find(ctx, o.FullRepositoryName, o.Number)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find PR %d in repo %s", o.Number, o.Repository)
	}
	return pr, nil
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
	args := []string{"--file", o.PreviewHelmfile}
	if o.Debug {
		args = append(args, "--debug")
	}
	args = append(args, "sync")
	c := &cmdrunner.Command{
		Name: "helmfile",
		Args: args,
		Env:  envVars,
	}
	_, err := o.CommandRunner(c)
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
		return "", fmt.Errorf("Preview namespace %s is too long. Must be no more than 63 character", prNamespace)
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

	url := ""
	fn := func() error {
		for _, n := range appNames {
			url, _ = services.FindServiceURL(o.KubeClient, releaseNamespace, n)
			/*
				TODO support kserve too
				if url == "" {
					var err error
					url, _, err = kserving.FindServiceURL(kserveClient, kubeClient, o.Namespace, n)
					if err != nil {
					  return errors.Wrapf(err, "failed to ")
					}
				}
			*/
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
	}
	pipeline := fmt.Sprintf("%s/%s/%s", o.Owner, o.Repository, o.Branch)

	build := o.BuildNumber
	if pipeline != "" && build != "" {
		ns := o.Namespace
		name := naming.ToValidName(pipeline + "-" + build)

		jxClient := o.JXClient

		// lets see if we can update the pipeline
		acts := jxClient.JenkinsV1().PipelineActivities(ns)
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
				_, err = acts.PatchUpdate(a)
				if err != nil {
					log.Logger().Warnf("Failed to update PipelineActivities %s: %s", name, err)
				} else {
					log.Logger().Infof("Updating PipelineActivities %s which has status %s", name, string(a.Spec.Status))
				}
			}
		}

		log.Logger().Infof("TODO update PipelineActivity %s/%s", ns, name)

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

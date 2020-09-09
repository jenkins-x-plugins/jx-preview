package create

import (
	"os"
	"strconv"
	"time"

	"context"
	"fmt"

	"github.com/cenkalti/backoff"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/pkg/kube/services"
	"github.com/jenkins-x/jx-helpers/pkg/options"
	"github.com/jenkins-x/jx-helpers/pkg/scmhelpers"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned"
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
	Debug             bool
	PreviewClient     versioned.Interface
	KubeClient        kubernetes.Interface
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

	return o.helmfileSyncPreview(envVars)
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
func (o *Options) findPreviewURL() (string, error) {
	// TODO lets load these values from the helmfile?
	application := ""
	releaseName := ""
	app := naming.ToValidName(application)
	appNames := []string{app, releaseName, o.Namespace + "-preview", releaseName + "-" + app}

	url := ""
	fn := func() error {
		for _, n := range appNames {
			url, _ = services.FindServiceURL(o.KubeClient, o.Namespace, n)
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
	err := backoff.Retry(fn, bo)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find preview URL for app names %#v in timeout %v", appNames, o.PreviewURLTimeout)
	}
	return url, nil
}

func (o *Options) updatePipelineActivity(url string) error {
	if url == "" {
		return nil
	}
	if o.BuildNumber == "" {
		o.BuildNumber = os.Getenv("BUILD_NUMBER")
	}
	pipeline := fmt.Sprintf("%s/%s/%s", o.Owner, o.Repository, o.Branch)

	if pipeline != "" && o.BuildNumber != "" {
		ns := o.Namespace
		name := naming.ToValidName(pipeline + "-" + o.BuildNumber)

		/*
			// lets see if we can update the pipeline
			activities := jxClient.JenkinsV1().PipelineActivities(ns)
			key := &kube.PromoteStepActivityKey{
				PipelineActivityKey: kube.PipelineActivityKey{
					Name:     name,
					Pipeline: pipeline,
					Build:    build,
					GitInfo: &gits.GitRepository{
						Name:         o.Repository,
						Organisation: o.Owner,
					},
				},
			}
			jxClient, _, err = o.JXClient()
			if err != nil {
				return err
			}
			a, _, p, _, err := key.GetOrCreatePreview(jxClient, ns)
			if err == nil && a != nil && p != nil {
				updated := false
				if p.ApplicationURL == "" {
					p.ApplicationURL = url
					updated = true
				}
				if p.PullRequestURL == "" && o.PullRequestURL != "" {
					p.PullRequestURL = o.PullRequestURL
					updated = true
				}
				if updated {
					_, err = activities.PatchUpdate(a)
					if err != nil {
						log.Logger().Warnf("Failed to update PipelineActivities %s: %s", name, err)
					} else {
						log.Logger().Infof("Updating PipelineActivities %s which has status %s", name, string(a.Spec.Status))
					}
				}
			}
		*/

		log.Logger().Infof("TODO update PipelineActivity %s/%s", ns, name)

	} else {
		log.Logger().Warnf("No pipeline and build number available on $JOB_NAME and $BUILD_NUMBER so cannot update PipelineActivities with the preview URLs")
	}
	return nil
}

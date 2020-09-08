package create

import (
	"os"
	"strconv"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/pkg/options"
	"github.com/jenkins-x/jx-helpers/pkg/scmhelpers"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-preview/pkg/previews"
	"github.com/jenkins-x/jx-preview/pkg/rootcmd"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"context"
	"fmt"
)

var (
	cmdLong = templates.LongDesc(`
		Creates a preview
`)

	cmdExample = templates.Examples(`
		# creates a new preview environemnt
		%s create
	`)

	mandatoryEnvVars = []string{"APP_NAME", "VERSION", "DOCKER_REGISTRY", "DOCKER_REGISTRY_ORG"}
)

// Options the CLI options for
type Options struct {
	scmhelpers.Options

	PreviewHelmfile string
	Number          int
	PreviewClient   versioned.Interface
	Namespace       string
	Debug           bool
	CommandRunner   cmdrunner.CommandRunner
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

	preview, _, err := previews.GetOrCreatePreview(o.PreviewClient, o.Namespace, pr, o.PreviewHelmfile)
	if err != nil {
		return errors.Wrapf(err, "failed to upsert the Preview resource in namespace %s", o.Namespace)
	}
	log.Logger().Infof("upserted preview %s", preview.Name)

	return o.helmfileSyncPreview(pr, preview)
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
	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.DefaultCommandRunner
	}
	return nil
}

func (o *Options) discoverPullRequest() (*scm.PullRequest, error) {
	ctx := context.Background()
	pr, _, err := o.ScmClient.PullRequests.Find(ctx, o.Repository, o.Number)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find PR %d in repo %s", o.Number, o.Repository)
	}
	return pr, nil
}

func (o *Options) helmfileSyncPreview(pr *scm.PullRequest, preview *v1alpha1.Preview) error {
	envVars, err := o.createHelmfileEnvVars(pr, preview)
	if err != nil {
		return errors.Wrapf(err, "failed to create env vars")
	}
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
	_, err = o.CommandRunner(c)
	if err != nil {
		return errors.Wrapf(err, "failed to run helmfile sync")
	}
	return nil
}

func (o *Options) createHelmfileEnvVars(pr *scm.PullRequest, preview *v1alpha1.Preview) (map[string]string, error) {
	env := map[string]string{}
	for _, e := range mandatoryEnvVars {
		value := os.Getenv(e)
		if value == "" {
			return env, errors.Errorf("missing $%s environment variable", e)
		}
		env[e] = value
	}
	return env, nil

}

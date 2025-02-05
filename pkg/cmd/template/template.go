package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jenkins-x-plugins/jx-gitops/pkg/plugins"
	"github.com/jenkins-x-plugins/jx-gitops/pkg/variablefinders"
	"github.com/jenkins-x-plugins/jx-preview/pkg/cmd/create"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews"
	jxcore "github.com/jenkins-x/jx-api/v4/pkg/apis/core/v4beta1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"

	"github.com/spf13/cobra"
)

// Options containers the CLI options
type Options struct {
	create.Options

	HelmfileBinary string
	Requirements   *jxcore.RequirementsConfig
	envCloneDir    string
}

var (
	cmdLong = templates.LongDesc(`
		Display one or more preview environments.
`)

	cmdExample = templates.Examples(`
		# Display the preview template yaml on the console 
		jx preview template
	`)
)

// NewCmdPreviewTemplate creates the new command for: jx get env
func NewCmdPreviewTemplate() (*cobra.Command, *Options) {
	o := &Options{}
	cmd := &cobra.Command{
		Use:     "template",
		Short:   "Displays the output of 'helmfile template' for the preview",
		Aliases: []string{"tmp"},
		Long:    cmdLong,
		Example: cmdExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	o.Options.AddFlags(cmd)
	cmd.Flags().StringVarP(&o.HelmfileBinary, "helmfile-binary", "", "", "specifies the helmfile binary location to use. If not specified defaults to using the downloaded helmfile plugin")

	return cmd, o
}

// Run implements this command
func (o *Options) Run() error {
	if o.Number == 0 {
		o.Number = 1
	}
	if o.Version == "" {
		o.Version = "0.0.0-PR-1-1-SNAPSHOT"
	}
	err := o.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate options: %w", err)
	}
	if o.HelmfileBinary == "" {
		o.HelmfileBinary, err = plugins.GetHelmfileBinary(plugins.HelmfileVersion)
		if err != nil {
			return fmt.Errorf("failed to download helmfile plugin: %w", err)
		}
	}

	o.envCloneDir, err = previews.CreateJXValuesFile(o.GitClient, o.JXClient, o.Namespace, filepath.Dir(o.PreviewHelmfile), o.PreviewNamespace, o.GitUser, o.GitToken)
	if err != nil {
		return fmt.Errorf("failed to create the jx-values.yaml file: %w", err)
	}

	envVars, err := o.CreateHelmfileEnvVars(o.defaultEnvVar)
	if err != nil {
		return fmt.Errorf("failed to create env vars: %w", err)
	}
	envVars["VERSION"] = o.Version
	if envVars["PULL_NUMBER"] == "" {
		envVars["PULL_NUMBER"] = strconv.Itoa(o.Number)
	}

	err = o.helmfileTemplate(envVars)
	if err != nil {
		return fmt.Errorf("failed to run helmfile template: %w", err)
	}

	return nil
}

func (o *Options) helmfileTemplate(envVars map[string]string) error {
	log.Logger().Infof("passing env vars into helmfile: %#v", envVars)

	args := []string{"--file", o.PreviewHelmfile}
	if o.Debug {
		args = append(args, "--debug")
	}

	c := &cmdrunner.Command{
		Name: "helmfile",
		Args: append(args, "template"),
		Out:  os.Stdout,
		Err:  os.Stderr,
		Env:  envVars,
	}
	_, err := o.CommandRunner(c)
	if err != nil {
		return fmt.Errorf("failed to run helmfile template: %w", err)
	}
	return nil
}

func (o *Options) defaultEnvVar(name string) (string, error) {
	if o.envCloneDir == "" {
		return "", fmt.Errorf("no environment git clone dir")
	}
	var err error
	if o.Requirements == nil {
		o.Requirements, err = variablefinders.FindRequirements(o.GitClient, o.JXClient, o.Namespace, o.envCloneDir, o.Owner, o.Repository)
		if err != nil {
			return "", fmt.Errorf("failed to load requirements: %w", err)
		}
	}

	switch name {
	case "DOCKER_REGISTRY":
		return o.Requirements.Cluster.Registry, nil
	case "DOCKER_REGISTRY_ORG":
		return variablefinders.DockerRegistryOrg(o.Requirements, o.Owner)
	case "PREVIEW_NAMESPACE":
		return "preview-ns", nil
	default:
		return "", nil
	}
}

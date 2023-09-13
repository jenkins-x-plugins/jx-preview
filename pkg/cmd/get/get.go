package get

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jenkins-x-plugins/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews"
	"github.com/jenkins-x-plugins/jx-preview/pkg/rootcmd"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/table"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Options containers the CLI options
type Options struct {
	PreviewClient versioned.Interface
	Namespace     string

	Current bool
}

var (
	cmdLong = templates.LongDesc(`
		Display one or more preview environments.
`)

	cmdExample = templates.Examples(`
		# List all preview environments
		%s get

		# View the current preview environment URL
		# inside a CI pipeline
		%s get --current
	`)
)

// NewCmdGetPreview creates the new command for: jx get env
func NewCmdGetPreview() (*cobra.Command, *Options) {
	options := &Options{}
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Display one or more Previews",
		Aliases: []string{"list"},
		Long:    cmdLong,
		Example: fmt.Sprintf(cmdExample, rootcmd.BinaryName, rootcmd.BinaryName),
		Run: func(cmd *cobra.Command, args []string) {
			err := options.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().BoolVarP(&options.Current, "current", "c", false, "Output the URL of the current Preview application the current pipeline just deployed")
	return cmd, options
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	if o.Current {
		return o.CurrentPreviewURL()
	}

	ctx := context.Background()
	ns := o.Namespace
	resourceList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to list Previews in namespace %s", ns)
		}
	}

	resources := resourceList.Items
	previews.SortPreviews(resources)

	t := table.CreateTable(os.Stdout)
	t.AddRow("PULL REQUEST", "NAMESPACE", "APPLICATION")

	for k := range resources {
		preview := resources[k]
		t.AddRow(preview.Spec.PullRequest.URL, preview.Spec.Resources.Namespace, preview.Spec.Resources.URL)
	}
	t.Render()
	return nil
}

// Validate validates the inputs are valid
func (o *Options) Validate() error {
	var err error
	o.PreviewClient, o.Namespace, err = previews.LazyCreatePreviewClientAndNamespace(o.PreviewClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create Preview client")
	}
	return nil
}

func (o *Options) CurrentPreviewURL() error {
	ctx := context.Background()
	ns := o.Namespace
	resourceList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to list previews in namespace %s", ns)
		}
	}

	repoName := os.Getenv("REPO_NAME")
	prNumber, err := strconv.Atoi(os.Getenv("PULL_NUMBER"))
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve current preview in namespace %s", ns)
	}

	var currentPreview v1alpha1.Preview
	for _, preview := range resourceList.Items {
		if preview.Spec.PullRequest.Number == prNumber &&
			preview.Spec.PullRequest.Repository == repoName {
			currentPreview = preview
		}
	}
	if &currentPreview == nil {
		return fmt.Errorf("no current preview for %s on pull request #%s", repoName, prNumber)
	}

	t := table.CreateTable(os.Stdout)
	t.AddRow("PULL REQUEST", "NAMESPACE", "APPLICATION")
	t.AddRow(currentPreview.Spec.PullRequest.URL,
		currentPreview.Spec.Resources.Namespace,
		currentPreview.Spec.Resources.URL)

	t.Render()
	return nil
}

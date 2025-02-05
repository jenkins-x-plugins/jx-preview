package get

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jenkins-x-plugins/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x-plugins/jx-preview/pkg/common"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews"
	"github.com/jenkins-x-plugins/jx-preview/pkg/rootcmd"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/table"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Options containers the CLI options
type Options struct {
	scmhelpers.PullRequestOptions

	PreviewClient versioned.Interface
	Namespace     string
	LatestCommit  string
	OutputEnvVars map[string]string

	Current bool
	Wait    bool
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
	cmd.Flags().BoolVarP(&options.Wait, "wait", "w", false, "Waits for a preview deployment with commit hash that matches latest commit")
	return cmd, options
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate options: %w", err)
	}

	if o.Current {
		return o.CurrentPreviewURL()
	}

	ctx := context.Background()
	ns := o.Namespace
	resourceList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to list Previews in namespace %s: %w", ns, err)
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
		return fmt.Errorf("failed to create Preview client: %w", err)
	}

	return nil
}

func (o *Options) CurrentPreviewURL() error {
	err := o.ValidateCurrent()
	if err != nil {
		return fmt.Errorf("failed to validate current options: %w", err)
	}

	pr, err := o.DiscoverPullRequest()
	if err != nil {
		return fmt.Errorf("failed to read pull request options: %w", err)
	}
	o.LatestCommit = pr.Head.Sha

	var currentPreview *v1alpha1.Preview
	if o.Wait {
		currentPreview, err = o.waitForCommit()
		if err != nil {
			return fmt.Errorf("failed whilst waiting for commit: %w", err)
		}
	} else {
		previewList, err := o.listPreviews()
		currentPreview = o.getPreview(previewList.Items)
		if err != nil {
			return fmt.Errorf("failed whilst retrieving preview: %w", err)
		}
	}

	if currentPreview == nil {
		log.Logger().Infof("No preview found for PR %v", o.Number)
		return nil
	}

	t := table.CreateTable(os.Stdout)
	t.AddRow("PULL REQUEST", "NAMESPACE", "APPLICATION")
	t.AddRow(currentPreview.Spec.PullRequest.URL,
		currentPreview.Spec.Resources.Namespace,
		currentPreview.Spec.Resources.URL)

	t.Render()

	o.OutputEnvVars["PREVIEW_URL"] = currentPreview.Spec.Resources.URL
	o.OutputEnvVars["PREVIEW_NAME"] = currentPreview.Name
	o.OutputEnvVars["PREVIEW_NAMESPACE"] = currentPreview.Spec.Resources.Namespace
	o.OutputEnvVars["PREVIEW_PULL_REQUEST_URL"] = currentPreview.Spec.PullRequest.URL

	err = common.WriteOutputEnvVars(o.Dir, o.OutputEnvVars)
	if err != nil {
		return fmt.Errorf("failed to write output environment variables: %w", err)
	}

	return nil
}

func (o *Options) ValidateCurrent() error {
	if o.OutputEnvVars == nil {
		o.OutputEnvVars = map[string]string{}
	}
	o.DiscoverFromGit = true

	err := o.PullRequestOptions.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate pull request options: %w", err)
	}

	return nil
}

func (o *Options) listPreviews() (*v1alpha1.PreviewList, error) {
	ctx := context.Background()
	ns := o.Namespace
	resourceList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to list previews in namespace %s: %w", ns, err)
		}
	}
	return resourceList, nil
}

func (o *Options) waitForCommit() (*v1alpha1.Preview, error) {
	log.Logger().Infof("Waiting for preview with commit: %s\n", o.LatestCommit)

	for {
		previewList, err := o.listPreviews()
		if err != nil {
			return nil, err
		}

		preview := o.getPreview(previewList.Items)
		if err != nil {
			return nil, err
		}
		if preview != nil && preview.Spec.PullRequest.LatestCommit == o.LatestCommit &&
			preview.Spec.Resources.URL != "" {
			return preview, nil
		}

		time.Sleep(5 * time.Second)
	}
}

func (o *Options) getPreview(previews []v1alpha1.Preview) *v1alpha1.Preview {
	for i := 0; i < len(previews); i++ {
		if previews[i].Spec.PullRequest.Number == o.Number &&
			previews[i].Spec.PullRequest.Repository == o.Repository {
			return &previews[i]
		}
	}

	return nil
}

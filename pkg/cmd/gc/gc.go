package gc

import (
	"context"
	"fmt"
	"os"

	"github.com/jenkins-x-plugins/jx-preview/pkg/cmd/destroy"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews"
	"github.com/jenkins-x-plugins/jx-preview/pkg/rootcmd"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jenkins-x/jx-logging/v3/pkg/log"
)

// Options the command line options
type Options struct {
	destroy.Options

	Deleted       []string
	DestroyDrafts bool
	DryRun        bool
}

var (
	cmdLong = templates.LongDesc(`
		Garbage collect Jenkins X preview environments.

		If a pull request is merged or closed the associated preview
		environment will be deleted.

`)

	cmdExample = templates.Examples(`
		# garbage collect previews
		%s gc
`)
)

func NewCmdGCPreviews() (*cobra.Command, *Options) {
	options := &Options{}

	cmd := &cobra.Command{
		Use:     "gc",
		Short:   "Garbage collect Preview environments for closed or merged Pull Requests",
		Long:    cmdLong,
		Example: fmt.Sprintf(cmdExample, rootcmd.BinaryName),
		Run: func(_ *cobra.Command, _ []string) {
			err := options.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().BoolVarP(&options.DestroyDrafts, "gc-drafts", "", false, "Also garbage collect drafts")
	cmd.Flags().BoolVarP(&options.DryRun, "dry-run", "", false, "Don't garbage collect, just display which would be deleted")

	return cmd, options
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate options: %w", err)
	}

	ns := o.Namespace
	ctx := context.Background()
	resourceList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to list Previews in namespace %s: %w", ns, err)
		}
	}

	resources := resourceList.Items
	previews.SortPreviews(resources)

	if o.DryRun {
		log.Logger().Info("These previews are selected for destruction:")
	}
	defer func() {
		if o.DevDir != "" {
			err := os.RemoveAll(o.DevDir)
			if err != nil {
				log.Logger().Warnf("failed to remove %s: %s", o.DevDir, err)
			}
		}
	}()
	for k := range resources {
		preview := resources[k]
		name := preview.Name
		gitURL := preview.Spec.Source.CloneURL
		if gitURL == "" {
			log.Logger().Warnf("cannot GC preview %s as it has no spec.source.cloneURL", name)
			continue
		}
		prLink := preview.Spec.PullRequest.URL
		owner := preview.Spec.PullRequest.Owner
		if owner == "" {
			log.Logger().Warnf("cannot GC preview %s as it has no spec.pullRequest.owner", name)
			continue
		}
		repository := preview.Spec.PullRequest.Repository
		if repository == "" {
			log.Logger().Warnf("cannot GC preview %s as it has no spec.pullRequest.repository", name)
			continue
		}
		prNumber := preview.Spec.PullRequest.Number
		if prNumber <= 0 {
			log.Logger().Warnf("cannot GC preview %s as it has no spec.pullRequest.number", name)
			continue
		}

		so := &scmhelpers.Options{
			// lets avoid detecting the branch
			Branch:    "master",
			ScmClient: o.ScmClient,
			SourceURL: gitURL,
			Namespace: o.Namespace,
			JXClient:  o.JXClient,
		}
		err = so.Validate()
		if err != nil {
			return fmt.Errorf("failed to validate preview %s with source URL %s: %w", name, preview.Spec.Source.URL, err)
		}

		scmClient := so.ScmClient
		fullName := scm.Join(owner, repository)

		ctx := context.Background()
		pullRequest, _, err := scmClient.PullRequests.Find(ctx, fullName, prNumber)
		if err != nil {
			return fmt.Errorf("failed to query PullRequest %s: %w", prLink, err)
		}

		if pullRequest.Closed || pullRequest.Merged || (o.DestroyDrafts && pullRequest.Draft && !scmhelpers.ContainsLabel(pullRequest.Labels, "ok-to-test")) {
			if !o.DryRun {
				err = o.Destroy(name)
				if err != nil {
					return fmt.Errorf("failed to destroy preview environment %s: %v", name, err)
				}
			} else {
				log.Logger().Info(name)
			}
			o.Deleted = append(o.Deleted, name)
		}
	}
	if len(o.Deleted) == 0 {
		log.Logger().Debug("no preview environments to garbage collect where found")
		return nil
	}
	return nil
}

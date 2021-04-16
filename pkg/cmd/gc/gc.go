package gc

import (
	"context"
	"fmt"

	"github.com/jenkins-x-plugins/jx-preview/pkg/cmd/destroy"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"strings"

	"github.com/jenkins-x/jx-logging/v3/pkg/log"
)

// Options the command line options
type Options struct {
	destroy.Options

	Deleted []string
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

// NewCmd s a command object for the "step" command
func NewCmdGCPreviews() (*cobra.Command, *Options) {
	options := &Options{}

	cmd := &cobra.Command{
		Use:     "gc",
		Short:   "Garbage collect Preview environments for closed or merged Pull Requests",
		Long:    cmdLong,
		Example: cmdExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := options.Run()
			helper.CheckErr(err)
		},
	}
	return cmd, options
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	ns := o.Namespace
	ctx := context.Background()
	resourceList, err := o.PreviewClient.PreviewV1alpha1().Previews(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to list Previews in namespace %s", ns)
		}
	}

	resources := resourceList.Items
	previews.SortPreviews(resources)

	for _, preview := range resources {
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
			return errors.Wrapf(err, "failed to validate preview %s with source URL %s", name, preview.Spec.Source.URL)
		}

		scmClient := so.ScmClient
		fullName := scm.Join(owner, repository)

		ctx := context.Background()
		pullRequest, _, err := scmClient.PullRequests.Find(ctx, fullName, prNumber)
		if err != nil {
			return errors.Wrapf(err, "failed to query PullRequest %s", prLink)
		}

		lowerState := strings.ToLower(pullRequest.State)

		if strings.HasPrefix(lowerState, "clos") || strings.HasPrefix(lowerState, "merged") || strings.HasPrefix(lowerState, "superseded") || strings.HasPrefix(lowerState, "declined") {

			err = o.Destroy(name)
			if err != nil {
				return fmt.Errorf("failed to destroy preview environment %s: %v\n", name, err)
			}

			o.Deleted = append(o.Deleted, name)
		}
	}
	if len(o.Deleted) == 0 {
		log.Logger().Debug("no preview environments found")
		return nil
	}
	return nil
}

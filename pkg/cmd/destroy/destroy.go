package destroy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jenkins-x/jx-helpers/v3/pkg/files"

	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"

	"github.com/jenkins-x-plugins/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x-plugins/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x-plugins/jx-preview/pkg/previews"
	"github.com/jenkins-x-plugins/jx-preview/pkg/rootcmd"
	jxc "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/cli"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	cmdLong = templates.LongDesc(`
		Destroys a preview environment
`)

	cmdExample = templates.Examples(`
		# destroys a preview environment
		%s destroy jx-myorg-myapp-pr-4
	`)

	info = termcolor.ColorInfo
)

// Options the CLI options for
type Options struct {
	options.BaseOptions
	scmhelpers.Options // Only used for tests
	Names              []string
	Namespace          string
	Filter             string
	Dir                string
	GitUser            string // Only used for tests
	FailOnHelmError    bool
	SelectAll          bool
	PreviewClient      versioned.Interface
	KubeClient         kubernetes.Interface
	JXClient           jxc.Interface
	GitClient          gitclient.Interface
	CommandRunner      cmdrunner.CommandRunner
	Input              input.Interface
	DevDir             string
}

// NewCmdPreviewDestroy creates a command object for the command
func NewCmdPreviewDestroy() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "destroy",
		Short:   "Destroys a preview environment",
		Aliases: []string{"delete", "remove"},
		Long:    cmdLong,
		Example: fmt.Sprintf(cmdExample, rootcmd.BinaryName),
		Run: func(_ *cobra.Command, args []string) {
			o.Names = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Filter, "filter", "", "", "The filter to use to find previews to delete")
	cmd.Flags().StringVarP(&o.Dir, "dir", "", "", "The directory where to run the delete preview command - a git clone will be done on a temporary jx-git-xxx directory if this parameter is empty")
	cmd.Flags().BoolVarP(&o.SelectAll, "all", "", false, "Select all the previews that match filter by default")
	cmd.Flags().BoolVarP(&o.FailOnHelmError, "fail-on-helm", "", false, "If enabled do not try to remove the namespace or Preview resource if we fail to destroy helmfile resources")
	return cmd, o
}

// Run implements a helmfile based preview environment
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate options: %w", err)
	}

	if len(o.Names) == 0 && !o.BatchMode {
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

		var names []string
		for k := range resources {
			names = append(names, resources[k].Name)
		}

		o.Names, err = o.Input.SelectNamesWithFilter(names, "select preview(s) to delete: ", o.SelectAll, o.Filter, "pick the names of the previews to remove")
		if err != nil {
			return fmt.Errorf("failed to select names to delete: %w", err)
		}
	}
	if len(o.Names) == 0 {
		return fmt.Errorf("missing preview name")

	}

	defer func() {
		if o.DevDir != "" {
			err := os.RemoveAll(o.DevDir)
			if err != nil {
				log.Logger().WithError(err).Warnf("failed to remove %s", o.DevDir)
			}
		}
	}()
	for _, name := range o.Names {
		err = o.Destroy(name)
		if err != nil {
			return fmt.Errorf("failed to destroy preview %s: %w", name, err)
		}
	}
	return nil
}

// Destroy destroys a preview environment
func (o *Options) Destroy(name string) error {
	ns := o.Namespace

	log.Logger().Infof("destroying preview: %s in namespace %s", info(name), info(ns))

	ctx := context.Background()
	previewInterface := o.PreviewClient.PreviewV1alpha1().Previews(ns)

	preview, err := previewInterface.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to find preview %s in namespace %s: %w", name, ns, err)
	}

	if preview.Spec.DestroyCommand.Command != "" {
		previewNamespace := preview.Spec.Resources.Namespace

		previewPath := preview.Spec.DestroyCommand.Path
		if previewPath == "" {
			previewPath = "preview"
		}
		dir := o.Dir
		if dir == "" {
			dir, err = o.gitCloneSource(preview, previewPath)
			if err != nil {
				return fmt.Errorf("failed to git clone preview source: %w", err)
			}
			defer os.RemoveAll(dir)
		}

		fullPreviewPath := filepath.Join(dir, previewPath)
		exists, err := files.DirExists(fullPreviewPath)
		if err != nil {
			return fmt.Errorf("failed to check existence of preview directory %s: %w", fullPreviewPath, err)
		}

		if exists {
			o.DevDir, err = previews.CreateJXValuesFileWithCloneDir(o.GitClient, o.JXClient, o.Namespace, fullPreviewPath, previewNamespace, o.GitUser, o.GitToken, o.DevDir)
			if err != nil {
				log.Logger().WithError(err).Warnf("failed to create the jx-values.yaml file")
			}
		}

		err = o.runDeletePreviewCommand(preview, dir)
		if err != nil {
			if o.FailOnHelmError {
				return fmt.Errorf("failed to delete preview resources: %w", err)
			}
			log.Logger().WithError(err).Warnf("could not delete preview resources")
		}
	}

	err = o.deletePreviewNamespace(preview)
	if err != nil {
		return fmt.Errorf("failed to delete preview namespace: %w", err)
	}

	err = previewInterface.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete preview %s in namespace %s: %w", name, ns, err)
	}
	log.Logger().Infof("deleted preview: %s in namespace %s", info(name), info(ns))
	return nil
}

// Validate validates the inputs are valid
func (o *Options) Validate() error {
	err := o.BaseOptions.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate base options: %w", err)
	}
	if o.Input == nil {
		o.Input = inputfactory.NewInput(&o.BaseOptions)
	}
	o.PreviewClient, o.Namespace, err = previews.LazyCreatePreviewClientAndNamespace(o.PreviewClient, o.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create Preview client: %w", err)
	}

	o.KubeClient, err = kube.LazyCreateKubeClient(o.KubeClient)
	if err != nil {
		return fmt.Errorf("failed to create kube client: %w", err)
	}
	o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
	if err != nil {
		return fmt.Errorf("failed to create jx client: %w", err)
	}

	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.QuietCommandRunner
	}
	if o.GitClient == nil {
		o.GitClient = cli.NewCLIClient("", o.CommandRunner)
	}
	return nil
}

func (o *Options) runDeletePreviewCommand(preview *v1alpha1.Preview, dir string) error {
	destroyCmd := preview.Spec.DestroyCommand

	envVars := map[string]string{}

	for _, ev := range destroyCmd.Env {
		envVars[ev.Name] = ev.Value
	}
	if destroyCmd.Path != "" {
		dir = filepath.Join(dir, destroyCmd.Path)
	}
	c := &cmdrunner.Command{
		Name: destroyCmd.Command,
		Args: destroyCmd.Args,
		Dir:  dir,
		Env:  envVars,
	}
	_, err := o.CommandRunner(c)
	if err != nil {
		return fmt.Errorf("failed to run destroy command: %w", err)
	}
	return nil

}

func (o *Options) deletePreviewNamespace(preview *v1alpha1.Preview) error {
	ctx := context.Background()

	previewNamespace := preview.Spec.Resources.Namespace
	if previewNamespace == "" {
		return fmt.Errorf("no preview.Spec.PreviewNamespace is defined for preview %s", preview.Name)
	}

	namespaceInterface := o.KubeClient.CoreV1().Namespaces()
	_, err := namespaceInterface.Get(ctx, previewNamespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Logger().Infof("there is no preview namespace %s to be removed", info(previewNamespace))
			return nil
		}
		return fmt.Errorf("failed to find preview namespace %s: %w", previewNamespace, err)
	}

	err = namespaceInterface.Delete(ctx, previewNamespace, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete preview namespace %s: %w", previewNamespace, err)
	}
	log.Logger().Infof("deleted preview namespace %s", info(previewNamespace))
	return nil
}

func (o *Options) gitCloneSource(preview *v1alpha1.Preview, path string) (string, error) {
	if o.GitClient == nil {
		o.GitClient = cli.NewCLIClient("", o.CommandRunner)
	}
	spec := preview.Spec
	gitURL := spec.Source.CloneURL
	if gitURL == "" {
		return "", fmt.Errorf("no preview.Spec.Source.URL to clone for preview %s", preview.Name)
	}
	dir, err := gitclient.SparseCloneToDir(o.GitClient, gitURL, "", false, path)
	if err != nil {
		return "", err
	}
	if spec.PullRequest.LatestCommit != "" {
		err := gitclient.Checkout(o.GitClient, dir, spec.PullRequest.LatestCommit)
		if err != nil {
			log.Logger().WithError(err).Warnf("failed to checkout latest commit for preview %s", preview.Name)
		}
	}
	return dir, nil
}

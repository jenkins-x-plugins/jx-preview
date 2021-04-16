package destroy

import (
	"context"
	"path/filepath"

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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"fmt"
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
	scmhelpers.Options
	Names           []string
	Namespace       string
	PreviewHelmfile string
	Filter          string
	GitUser         string
	FailOnHelmError bool
	SelectAll       bool
	PreviewClient   versioned.Interface
	KubeClient      kubernetes.Interface
	JXClient        jxc.Interface
	GitClient       gitclient.Interface
	CommandRunner   cmdrunner.CommandRunner
	Input           input.Interface
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
		Run: func(cmd *cobra.Command, args []string) {
			o.Names = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.PreviewHelmfile, "file", "f", "", "Preview helmfile.yaml path to use. If not specified it is discovered in preview/helmfile.yaml and created from a template if needed")
	cmd.Flags().StringVarP(&o.Filter, "filter", "", "", "The filter to use to find a preview to delete")
	cmd.Flags().StringVarP(&o.GitUser, "git-user", "", "", "The user name to git clone the environment repository")
	cmd.Flags().BoolVarP(&o.SelectAll, "all", "", false, "Select all the filters by default to remove")
	cmd.Flags().BoolVarP(&o.FailOnHelmError, "fail-on-helm", "", false, "If enabled do not try to remove the namespace or Preview resource if we fail to destroy helmfile resources")
	return cmd, o
}

// Run implements a helmfile based preview environment
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	if len(o.Names) == 0 && !o.BatchMode {
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

		var names []string
		for _, r := range resources {
			names = append(names, r.Name)
		}

		o.Names, err = o.Input.SelectNamesWithFilter(names, "select preview(s) to delete: ", o.SelectAll, o.Filter, "pick the names of the previews to remove")
		if err != nil {
			return errors.Wrapf(err, "failed to select names to delete")
		}
	}
	if len(o.Names) == 0 {
		return errors.Errorf("missing preview name")

	}

	for _, name := range o.Names {
		err = o.Destroy(name)
		if err != nil {
			return errors.Wrapf(err, "failed to destroy preview %s", name)
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
		return errors.Wrapf(err, "failed to find preview %s in namespace %s", name, ns)
	}

	dir, err := o.gitCloneSource(preview)
	if err != nil {
		return errors.Wrapf(err, "failed to git clone preview source")
	}

	previewNamespace := preview.Spec.Resources.Namespace

	if o.PreviewHelmfile == "" {
		o.PreviewHelmfile, err = previews.DiscoverHelmfile(dir)
		if err != nil {
			return errors.Wrapf(err, "failed to discover the helmfile in the dir")
		}
	}
	err = previews.CreateJXValuesFile(o.GitClient, o.JXClient, o.Namespace, o.PreviewHelmfile, previewNamespace, o.GitUser, o.GitToken)
	if err != nil {
		return errors.Wrapf(err, "failed to create the jx-values.yaml file")
	}

	err = o.runDeletePreviewCommand(preview, dir)
	if err != nil {
		if o.FailOnHelmError {
			return errors.Wrapf(err, "failed to delete preview resources")
		}
		log.Logger().Warnf("could not delete preview resources: %s", err.Error())
	}

	err = o.deletePreviewNamespace(preview)
	if err != nil {
		return errors.Wrapf(err, "failed to delete preview namespace")
	}

	err = previewInterface.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to delete preview %s in namespace %s", name, ns)
	}
	log.Logger().Infof("deleted preview: %s in namespace %s", info(name), info(ns))
	return nil
}

// Validate validates the inputs are valid
func (o *Options) Validate() error {
	err := o.BaseOptions.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate base options")
	}
	if o.Input == nil {
		o.Input = inputfactory.NewInput(&o.BaseOptions)
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
		return errors.Wrapf(err, "failed to run destroy command")
	}
	return nil

}

func (o *Options) deletePreviewNamespace(preview *v1alpha1.Preview) error {
	ctx := context.Background()

	previewNamespace := preview.Spec.Resources.Namespace
	if previewNamespace == "" {
		return errors.Errorf("no preview.Spec.PreviewNamespace is defined for preview %s", preview.Name)
	}

	namespaceInterface := o.KubeClient.CoreV1().Namespaces()
	_, err := namespaceInterface.Get(ctx, previewNamespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Logger().Infof("there is no preview namespace %s to be removed", info(previewNamespace))
			return nil
		}
		return errors.Wrapf(err, "failed to find preview namespace %s", previewNamespace)
	}

	err = namespaceInterface.Delete(ctx, previewNamespace, metav1.DeleteOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to delete preview namespace %s", previewNamespace)
	}
	log.Logger().Infof("deleted preview namespace %s", info(previewNamespace))
	return nil
}

func (o *Options) gitCloneSource(preview *v1alpha1.Preview) (string, error) {
	if o.GitClient == nil {
		o.GitClient = cli.NewCLIClient("", o.CommandRunner)
	}
	gitURL := preview.Spec.Source.URL
	if gitURL == "" {
		return "", errors.Errorf("no preview.Spec.Source.URL to clone for preview %s", preview.Name)
	}
	return gitclient.CloneToDir(o.GitClient, gitURL, "")
}

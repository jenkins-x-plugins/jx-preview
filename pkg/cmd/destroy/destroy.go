package destroy

import (
	"context"
	"path/filepath"

	"github.com/jenkins-x/jx-gitops/pkg/cmd/git/get"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/cli"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/jx-preview/pkg/apis/preview/v1alpha1"
	"github.com/jenkins-x/jx-preview/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-preview/pkg/previews"
	"github.com/jenkins-x/jx-preview/pkg/rootcmd"
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
	scmhelpers.PullRequestOptions

	Names           []string
	Namespace       string
	PreviewClient   versioned.Interface
	PreviewHelmfile string
	KubeClient      kubernetes.Interface
	GitClient       gitclient.Interface
	CommandRunner   cmdrunner.CommandRunner
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
	return cmd, o
}

// Run implements a helmfile based preview environment
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	if len(o.Names) == 0 {
		return errors.Errorf("missing preview name")
	}

	for _, name := range o.Names {
		err = o.Destroy(name)
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

	err = o.createJXValuesFile()
	if err != nil {
		return errors.Wrapf(err, "failed to create the jx-values.yaml file")
	}

	err = o.runDeletePreviewCommand(preview, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to delete preview resources")
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
	var err error
	o.PreviewClient, o.Namespace, err = previews.LazyCreatePreviewClientAndNamespace(o.PreviewClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create Preview client")
	}

	o.KubeClient, err = kube.LazyCreateKubeClient(o.KubeClient)
	if err != nil {
		return errors.Wrapf(err, "failed to create kube client")
	}

	err = o.DiscoverPreviewHelmfile()
	if err != nil {
		return errors.Wrapf(err, "failed to discover the preview helmfile")
	}

	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.DefaultCommandRunner
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

func (o *Options) createJXValuesFile() error {
	_, getOpts := get.NewCmdGitGet()

	getOpts.Options = o.Options
	getOpts.Env = "dev"
	getOpts.Path = "jx-values.yaml"
	//getOpts.JXClient = o.JXClient
	getOpts.Namespace = o.Namespace
	fileName := filepath.Join(filepath.Dir(o.PreviewHelmfile), "jx-values.yaml")
	getOpts.To = fileName
	err := getOpts.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to get the file %s from Environment %s", getOpts.Path, getOpts.Env)
	}

	// // lets modify the ingress sub domain
	// m := map[string]interface{}{}
	// err = yamls.LoadFile(fileName, &m)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to load file %s", fileName)
	// }
	// subDomain := "-" + o.PreviewNamespace + "."
	// log.Logger().Infof("using ingress sub domain %s", info(subDomain))
	// maps.SetMapValueViaPath(m, "jxRequirements.ingress.namespaceSubDomain", subDomain)
	// err = yamls.SaveFile(m, fileName)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to save file %s", fileName)
	// }
	return nil
}

// DiscoverPreviewHelmfile if there is no helmfile configured
// lets find the charts folder and default the preview helmfile to that
// then generate a helmfile.yaml if its missing
func (o *Options) DiscoverPreviewHelmfile() error {
	if o.PreviewHelmfile == "" {
		chartsDir := filepath.Join(o.Dir, "charts")
		exists, err := files.DirExists(chartsDir)
		if err != nil {
			return errors.Wrapf(err, "failed to check if dir %s exists", chartsDir)
		}
		if !exists {
			chartsDir = filepath.Join(o.Dir, "..")
			exists, err = files.DirExists(chartsDir)
			if err != nil {
				return errors.Wrapf(err, "failed to check if dir %s exists", chartsDir)
			}
			if !exists {
				return errors.Wrapf(err, "could not detect the helm charts folder in dir %s", o.Dir)
			}
		}

		o.PreviewHelmfile = filepath.Join(chartsDir, "..", "preview", "helmfile.yaml")
	}

	exists, err := files.FileExists(o.PreviewHelmfile)
	if err != nil {
		return errors.Wrapf(err, "failed to check for file %s", o.PreviewHelmfile)
	}
	if exists {
		return nil
	}

	// // lets make the preview dir
	// previewDir := filepath.Dir(o.PreviewHelmfile)
	// parentDir := filepath.Dir(previewDir)
	// relDir, err := filepath.Rel(o.Dir, parentDir)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to find preview dir in %s of %s", o.Dir, parentDir)
	// }
	// err = os.MkdirAll(parentDir, files.DefaultDirWritePermissions)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to make preview dir %s", parentDir)
	// }

	// // now lets grab the template preview
	// c := &cmdrunner.Command{
	// 	Dir:  o.Dir,
	// 	Name: "kpt",
	// 	Args: []string{"pkg", "get", "https://github.com/jenkins-x/jx3-gitops-template.git/charts/preview", relDir},
	// }
	// _, err = o.CommandRunner(c)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to get the preview helmfile: %s via kpt", o.PreviewHelmfile)
	// }

	// // lets add the files
	// if o.GitClient == nil {
	// 	o.GitClient = cli.NewCLIClient("", o.CommandRunner)
	// }
	// _, err = o.GitClient.Command(o.Dir, "add", "*")
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to add the preview helmfile files to git")
	// }
	// _, err = o.GitClient.Command(o.Dir, "commit", "-a", "-m", "fix: add preview helmfile")
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to commit the preview helmfile files to git")
	// }

	// // lets push the changes to git
	// _, po := push.NewCmdPullRequestPush()
	// po.CommandRunner = o.CommandRunner
	// po.ScmClient = o.ScmClient
	// po.SourceURL = o.SourceURL
	// po.Number = o.Number
	// po.Branch = o.PullRequestBranch

	// err = po.Run()
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to push the changes to git")
	// }
	return nil
}

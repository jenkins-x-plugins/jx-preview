package previews

import (
	"path/filepath"

	jxc "github.com/jenkins-x/jx-api/v3/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-gitops/pkg/cmd/git/get"
	"github.com/jenkins-x/jx-helpers/v3/pkg/maps"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
)

var info = termcolor.ColorInfo

// CreateJXValuesFile creates the jx-values.yaml file from the dev environment
func CreateJXValuesFile(scmOptions scmhelpers.Options, jxClient jxc.Interface, namespace, helmfile, previewNamespace string) error {
	_, getOpts := get.NewCmdGitGet()

	getOpts.Options = scmOptions
	getOpts.Env = "dev"
	getOpts.Path = "jx-values.yaml"
	getOpts.JXClient = jxClient
	getOpts.Namespace = namespace
	dir := filepath.Dir(helmfile)
	getOpts.Dir = dir
	fileName := filepath.Join(dir, "jx-values.yaml")
	getOpts.To = fileName
	err := getOpts.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to get the file %s from Environment %s", getOpts.Path, getOpts.Env)
	}

	// lets modify the ingress sub domain
	m := map[string]interface{}{}
	err = yamls.LoadFile(fileName, &m)
	if err != nil {
		return errors.Wrapf(err, "failed to load file %s", fileName)
	}
	subDomain := "-" + previewNamespace + "."
	log.Logger().Infof("using ingress sub domain %s", info(subDomain))
	maps.SetMapValueViaPath(m, "jxRequirements.ingress.namespaceSubDomain", subDomain)
	err = yamls.SaveFile(m, fileName)
	if err != nil {
		return errors.Wrapf(err, "failed to save file %s", fileName)
	}
	return nil
}

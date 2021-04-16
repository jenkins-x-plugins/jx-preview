package previews

import (
	"path/filepath"

	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/loadcreds"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"

	jxc "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/maps"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
)

var info = termcolor.ColorInfo

// CreateJXValuesFile creates the jx-values.yaml file from the dev environment
func CreateJXValuesFile(gitter gitclient.Interface, jxClient jxc.Interface, namespace, helmfile, previewNamespace, gitUser, gitToken string) error {
	dir := filepath.Dir(helmfile)
	fileName := filepath.Join(dir, "jx-values.yaml")

	devEnv, err := jxenv.GetDevEnvironment(jxClient, namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to find the dev environment in namespace %s", namespace)
	}
	if devEnv == nil {
		return errors.Errorf("not nto find the dev Environment in namespace %s", namespace)
	}

	url := devEnv.Spec.Source.URL
	if url == "" {
		return errors.Errorf("environment %s does not have a source URL", devEnv.Name)
	}
	if gitUser == "" || gitToken == "" {
		creds, err := loadcreds.LoadGitCredential()
		if err != nil {
			return errors.Wrapf(err, "failed to load git credentials")
		}

		gitInfo, err := giturl.ParseGitURL(url)
		if err != nil {
			return errors.Wrapf(err, "failed to parse git URL %s", url)
		}
		gitServerURL := gitInfo.HostURL()
		serverCreds := loadcreds.GetServerCredentials(creds, gitServerURL)

		if gitUser == "" {
			gitUser = serverCreds.Username
		}
		if gitToken == "" {
			gitToken = serverCreds.Password
		}
		if gitToken == "" {
			gitToken = serverCreds.Token
		}

		if gitUser == "" {
			return errors.Errorf("could not find git user for git server %s", gitServerURL)
		}
		if gitToken == "" {
			return errors.Errorf("could not find git token for git server %s", gitServerURL)
		}
	}

	gitCloneURL := url
	if gitUser != "" && gitToken != "" {
		gitCloneURL, err = stringhelpers.URLSetUserPassword(url, gitUser, gitToken)
		if err != nil {
			return errors.Wrapf(err, "failed to add user and token to git url %s", url)
		}
	}

	cloneDir, err := gitclient.CloneToDir(gitter, gitCloneURL, "")
	if err != nil {
		return errors.Wrapf(err, "failed to clone URL %s", gitCloneURL)
	}

	path := filepath.Join(cloneDir, "helmfiles", "jx", "jx-values.yaml")

	exists, err := files.FileExists(path)
	if err != nil {
		return errors.Wrapf(err, "failed to check if file %s exists in clone of %s", path, url)
	}
	if !exists {
		return errors.Wrapf(err, "file %s does not exist in clone of %s", path, url)
	}

	// lets modify the ingress sub domain
	m := map[string]interface{}{}
	err = yamls.LoadFile(path, &m)
	if err != nil {
		return errors.Wrapf(err, "failed to load file %s", path)
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

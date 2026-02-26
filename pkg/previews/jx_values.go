package previews

import (
	"fmt"
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
)

var info = termcolor.ColorInfo

// CreateJXValuesFile creates the jx-values.yaml file from the dev environment
func CreateJXValuesFile(gitter gitclient.Interface, jxClient jxc.Interface, namespace, dir, previewNamespace, gitUser, gitToken string) (string, error) {
	return CreateJXValuesFileWithCloneDir(gitter, jxClient, namespace, dir, previewNamespace, gitUser, gitToken, "")
}

// CreateJXValuesFileWithCloneDir creates the jx-values.yaml file from the dev environment, reusing file in cloneDir if set
func CreateJXValuesFileWithCloneDir(gitter gitclient.Interface, jxClient jxc.Interface, namespace, dir, previewNamespace, gitUser, gitToken, cloneDir string) (string, error) {
	fileName := filepath.Join(dir, "jx-values.yaml")

	jxValuesDir := filepath.Join("helmfiles", "jx")

	if cloneDir == "" {
		devEnv, err := jxenv.GetDevEnvironment(jxClient, namespace)
		if err != nil {
			return "", fmt.Errorf("failed to find the dev environment in namespace %s: %w", namespace, err)
		}
		if devEnv == nil {
			return "", fmt.Errorf("cannot find the dev environment in namespace %s", namespace)
		}

		url := devEnv.Spec.Source.URL
		if url == "" {
			return "", fmt.Errorf("environment %s does not have a source URL", devEnv.Name)
		}
		if gitUser == "" || gitToken == "" {
			creds, err := loadcreds.LoadGitCredential()
			if err != nil {
				return "", fmt.Errorf("failed to load git credentials: %w", err)
			}

			gitInfo, err := giturl.ParseGitURL(url)
			if err != nil {
				return "", fmt.Errorf("failed to parse git URL %s: %w", url, err)
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
				return "", fmt.Errorf("could not find git user for git server %s", gitServerURL)
			}
			if gitToken == "" {
				return "", fmt.Errorf("could not find git token for git server %s", gitServerURL)
			}
		}

		gitCloneURL := url
		if gitUser != "" && gitToken != "" {
			gitCloneURL, err = stringhelpers.URLSetUserPassword(url, gitUser, gitToken)
			if err != nil {
				return "", fmt.Errorf("failed to add user and token to git url %s: %w", url, err)
			}
		}

		cloneDir, err = gitclient.SparseCloneToDir(gitter, gitCloneURL, "", true, jxValuesDir)
		if err != nil {
			return "", fmt.Errorf("failed to clone URL %s: %w", gitCloneURL, err)
		}
	}

	path := filepath.Join(cloneDir, jxValuesDir, "jx-values.yaml")

	exists, err := files.FileExists(path)
	if err != nil {
		return cloneDir, fmt.Errorf("failed to check if file %s exists in cluster repo: %w", path, err)
	}
	if !exists {
		return cloneDir, fmt.Errorf("file %s does not exist in cluster repo", path)
	}

	// lets modify the ingress sub domain
	m := map[string]interface{}{}
	err = yamls.LoadFile(path, &m)
	if err != nil {
		return cloneDir, fmt.Errorf("failed to load file %s: %w", path, err)
	}
	subDomain := "-" + previewNamespace + "."
	log.Logger().Infof("using ingress sub domain %s", info(subDomain))
	maps.SetMapValueViaPath(m, "jxRequirements.ingress.namespaceSubDomain", subDomain)
	err = yamls.SaveFile(m, fileName)
	if err != nil {
		return cloneDir, fmt.Errorf("failed to save file %s: %w", fileName, err)
	}
	return cloneDir, nil
}

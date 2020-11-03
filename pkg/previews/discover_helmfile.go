package previews

import (
	"path/filepath"

	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/pkg/errors"
)

// DiscoverHelmfile discovers the helmfile in the gitven directory
func DiscoverHelmfile(dir string) (string, error) {
	chartsDir := filepath.Join(dir, "charts")
	exists, err := files.DirExists(chartsDir)
	if err != nil {
		return "", errors.Wrapf(err, "failed to check if dir %s exists", chartsDir)
	}
	if !exists {
		chartsDir = filepath.Join(dir, "..")
		exists, err = files.DirExists(chartsDir)
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if dir %s exists", chartsDir)
		}
		if !exists {
			return "helmfile", errors.Wrapf(err, "could not detect the helm charts folder in dir %s", dir)
		}
	}

	helmfile := filepath.Join(chartsDir, "..", "preview", "helmfile.yaml")
	exists, err = files.FileExists(helmfile)
	if err != nil {
		return helmfile, errors.Wrapf(err, "failed to check for file %s", helmfile)
	}
	if exists {
		return helmfile, nil
	}
	return "", nil

}

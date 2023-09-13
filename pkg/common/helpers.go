package common

import (
	"fmt"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"strings"
)

// BinaryName the binary name to use in help docs
var BinaryName string

// TopLevelCommand the top level command name
var TopLevelCommand string

func init() {
	BinaryName = os.Getenv("BINARY_NAME")
	if BinaryName == "" {
		BinaryName = "jx remote"
	}
	TopLevelCommand = os.Getenv("TOP_LEVEL_COMMAND")
	if TopLevelCommand == "" {
		TopLevelCommand = "jx remote"
	}
}

var info = termcolor.ColorInfo

func WriteOutputEnvVars(currentDir string, outputEnvVars map[string]string) error {
	path := filepath.Join(currentDir, ".jx", "variables.sh")

	text := ""
	exists, err := files.FileExists(path)
	if err != nil {
		return errors.Wrapf(err, "failed to check for file exist %s", path)
	}
	if exists {
		data, err := os.ReadFile(path)
		if err != nil {
			return errors.Wrapf(err, "failed to read %s", path)
		}
		text = string(data)
	}

	buf := strings.Builder{}
	buf.WriteString("# preview environment variables\n")
	for k, v := range outputEnvVars {
		buf.WriteString(fmt.Sprintf("export %s=%q\n", k, v))
	}
	if text != "" {
		buf.WriteString("\n\n")
		buf.WriteString(text)
	}
	text = buf.String()

	// make sure dir exists
	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, files.DefaultDirWritePermissions)
	if err != nil {
		return errors.Wrapf(err, "failed to make dir %s", dir)
	}

	err = os.WriteFile(path, []byte(text), files.DefaultFileWritePermissions)
	if err != nil {
		return errors.Wrapf(err, "failed to save file %s", path)
	}

	log.Logger().Infof("wrote preview environment variables to %s", info(path))
	return nil
}

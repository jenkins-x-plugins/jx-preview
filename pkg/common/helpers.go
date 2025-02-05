package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
)

var info = termcolor.ColorInfo

func WriteOutputEnvVars(currentDir string, outputEnvVars map[string]string) error {
	path := filepath.Join(currentDir, ".jx", "variables.sh")

	text := ""
	exists, err := files.FileExists(path)
	if err != nil {
		return fmt.Errorf("failed to check for file exist %s: %w", path, err)
	}
	if exists {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
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
		return fmt.Errorf("failed to make dir %s: %w", dir, err)
	}

	err = os.WriteFile(path, []byte(text), files.DefaultFileWritePermissions)
	if err != nil {
		return fmt.Errorf("failed to save file %s: %w", path, err)
	}

	log.Logger().Infof("wrote preview environment variables to %s", info(path))
	return nil
}

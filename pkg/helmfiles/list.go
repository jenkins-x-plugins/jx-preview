package helmfiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
)

// HelmRelease the output from listing the releases
type HelmRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Enabled   bool   `json:"enabled"`
	Labels    string `json:"labels"`
}

// ListReleases lists the releases in the helmfile
func ListReleases(runner cmdrunner.CommandRunner, file string, env map[string]string) ([]HelmRelease, error) {
	args := []string{"--file", file, "list", "--output", "json"}
	var b bytes.Buffer
	c := &cmdrunner.Command{
		Name: "helmfile",
		Args: args,
		Env:  env,
		Out:  &b,
	}
	_, err := runner(c)
	if err != nil {
		return nil, fmt.Errorf("failed to run %s: %w", c.CLI(), err)
	}
	output := strings.TrimSpace(b.String())
	if output == "" {
		return nil, nil
	}
	var answer []HelmRelease

	err = json.Unmarshal([]byte(output), &answer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %s: %w", output, err)
	}
	return answer, nil
}

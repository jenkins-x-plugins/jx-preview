package helmfiles

import (
	"encoding/json"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/pkg/errors"
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
	c := &cmdrunner.Command{
		Name: "helmfile",
		Args: args,
		Env:  env,
	}
	output, err := runner(c)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to run %s", c.CLI())
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}
	answer := []HelmRelease{}

	err = json.Unmarshal([]byte(output), &answer)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse JSON: %s", output)
	}
	return answer, nil
}

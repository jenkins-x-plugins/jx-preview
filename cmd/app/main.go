// +build !windows

package app

import (
	"github.com/jenkins-x-plugins/jx-preview/pkg/cmd"
)

// Run runs the command, if args are not nil they will be set on the command
func Run(args []string) error {
	rootCmd := cmd.Main()
	if args != nil {
		args = args[1:]
		rootCmd.SetArgs(args)
	}
	return rootCmd.Execute()
}

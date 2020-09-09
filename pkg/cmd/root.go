package cmd

import (
	"github.com/jenkins-x/jx-helpers/pkg/cobras"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-preview/pkg/cmd/create"
	"github.com/jenkins-x/jx-preview/pkg/cmd/destroy"
	"github.com/jenkins-x/jx-preview/pkg/cmd/version"
	"github.com/jenkins-x/jx-preview/pkg/rootcmd"
	"github.com/spf13/cobra"
)

// Main creates the new command
func Main() *cobra.Command {
	cmd := &cobra.Command{
		Use:   rootcmd.TopLevelCommand,
		Short: "Preview commands",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				log.Logger().Errorf(err.Error())
			}
		},
	}
	cmd.AddCommand(cobras.SplitCommand(create.NewCmdPreviewCreate()))
	cmd.AddCommand(cobras.SplitCommand(destroy.NewCmdPreviewDestroy()))
	cmd.AddCommand(cobras.SplitCommand(version.NewCmdVersion()))
	return cmd
}

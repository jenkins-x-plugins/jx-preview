package cmd

import (
	"github.com/jenkins-x/jx-helpers/pkg/cobras"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-preview/pkg/cmd/destroy"
	"github.com/jenkins-x/jx-preview/pkg/cmd/preview"
	"github.com/jenkins-x/jx-preview/pkg/cmd/version"
	"github.com/jenkins-x/jx-preview/pkg/rootcmd"
	"github.com/spf13/cobra"
)

// Main creates the new command
func Main() *cobra.Command {
	createCmd := preview.NewCmdPreviewLegacy()

	cmd := &cobra.Command{
		Use:   rootcmd.TopLevelCommand,
		Short: "Preview commands",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) <= 1 {
				log.Logger().Info("aliasing this to the create command")
				err := createCmd.ParseFlags(args)
				if err != nil {
					log.Logger().Errorf(err.Error())
					return
				}
				createCmd.Run(cmd, args)
				return
			}
			err := cmd.Help()
			if err != nil {
				log.Logger().Errorf(err.Error())
			}
		},
		DisableFlagParsing: true,
	}
	cmd.AddCommand(createCmd)
	cmd.AddCommand(cobras.SplitCommand(destroy.NewCmdPreviewDestroy()))
	cmd.AddCommand(cobras.SplitCommand(version.NewCmdVersion()))
	return cmd
}

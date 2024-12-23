package cli

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"sigs.k8s.io/network-policy-api/policy-assistant/pkg/utils"
)

func RunRootCommand() {
	command := SetupRootCommand()
	if err := errors.Wrapf(command.Execute(), "run root command"); err != nil {
		logrus.Fatalf("unable to run root command: %+v", err)
		os.Exit(1)
	}
}

type RootFlags struct {
	Verbosity string
}

func SetupRootCommand() *cobra.Command {
	flags := &RootFlags{}
	command := &cobra.Command{
		Use:   "policy-assistant",
		Short: "policy assistant for developing and troubleshooting any kubernetes network policy (NPv1, ANP, or BANP)",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetUpLogger(flags.Verbosity)
		},
	}

	command.PersistentFlags().StringVarP(&flags.Verbosity, "verbosity", "v", "info", "log level; one of [info, debug, trace, warn, error, fatal, panic]")

	command.AddCommand(SetupAnalyzeCommand())
	//command.AddCommand(SetupCompareCommand())
	command.AddCommand(SetupGenerateCommand())
	command.AddCommand(SetupProbeCommand())
	command.AddCommand(SetupVersionCommand())

	return command
}

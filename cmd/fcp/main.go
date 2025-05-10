package main

import (
	"os"

	"go.funccloud.dev/fcp/internal/cmd"
	"k8s.io/component-base/cli"
	"k8s.io/component-base/logs"
	"k8s.io/kubectl/pkg/cmd/util"

	// Import to initialize client auth plugins.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	logs.GlogSetter(cmd.GetLogVerbosity(os.Args)) // nolint:errcheck
	command := cmd.NewDefaultFCPCommand()
	if err := cli.RunNoErrOutput(command); err != nil {
		// Pretty-print the error and exit with an error.
		util.CheckErr(err)
	}
}

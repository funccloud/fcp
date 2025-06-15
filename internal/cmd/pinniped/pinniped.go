package pinniped

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	pinnipedcmd "go.pinniped.dev/cmd/pinniped/cmd"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/util/i18n"
)

//nolint:gochecknoinits
func init() {
	// browsers like chrome like to write to our std out which breaks our JSON ExecCredential output
	// thus we redirect the browser's std out to our std err
	browser.Stdout = os.Stderr
}

func ensureConciergeMode(runEArgs []string) []string {
	foundConciergeMode := false
	for _, arg := range runEArgs {
		if arg == "--concierge-mode=ImpersonationProxy" {
			foundConciergeMode = true
			break
		}
	}
	if !foundConciergeMode {
		insertIndex := 2 // Default after "get kubeconfig"
		for i, arg := range runEArgs {
			if i > 1 && arg[0] == '-' { // Found the first flag
				insertIndex = i
				break
			}
			if i == len(runEArgs)-1 { // No flags found, append at the end
				insertIndex = len(runEArgs)
			}
		}
		tempArgs := make([]string, 0, len(runEArgs)+1)
		tempArgs = append(tempArgs, runEArgs[:insertIndex]...)
		tempArgs = append(tempArgs, "--concierge-mode=ImpersonationProxy")
		tempArgs = append(tempArgs, runEArgs[insertIndex:]...)
		return tempArgs
	}
	return runEArgs
}

func ensureConciergeEndpoint(runEArgs []string) error {
	foundConciergeEndpoint := false
	for _, arg := range runEArgs {
		if arg == "--concierge-endpoint" || (len(arg) > 21 && arg[:21] == "--concierge-endpoint=") {
			foundConciergeEndpoint = true
			break
		}
	}
	if !foundConciergeEndpoint {
		return fmt.Errorf("the --concierge-endpoint flag is required for 'get kubeconfig'")
	}
	return nil
}

func processKubeconfigOutput(outputBuf *bytes.Buffer, streams genericiooptions.IOStreams) error {
	if outputBuf.Len() == 0 {
		return nil
	}

	cfg, err := clientcmd.Load(outputBuf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig output: %w", err)
	}

	for _, authInfo := range cfg.AuthInfos {
		if authInfo.Exec != nil {
			if authInfo.Exec.Args == nil {
				authInfo.Exec.Args = []string{}
			}
			if len(authInfo.Exec.Args) == 0 || authInfo.Exec.Args[0] != "pinniped" {
				authInfo.Exec.Args = append([]string{"pinniped"}, authInfo.Exec.Args...)
				authInfo.Exec.InstallHint = "Ensure FuncCloud Platform CLI is installed and configured for authentication."
			}
		}
	}

	// Use clientcmd.Write to serialize the config. This will convert it to the v1 format
	// which uses arrays for clusters, users, and contexts, as expected by kubectl.
	modifiedConfigBytes, err := clientcmd.Write(*cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize modified kubeconfig to v1 format: %w", err)
	}

	if _, err := streams.Out.Write(modifiedConfigBytes); err != nil {
		return fmt.Errorf("failed to write output to stream: %w", err)
	}
	return nil
}

func handleGetKubeconfigCmd(runEArgs []string, streams genericiooptions.IOStreams) error {
	runEArgs = ensureConciergeMode(runEArgs)
	// Update os.Args as well, as pinniped's cmd.Execute() reads from it
	os.Args = append([]string{"pinniped"}, runEArgs...)

	if err := ensureConciergeEndpoint(runEArgs); err != nil {
		return err
	}

	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		return fmt.Errorf("failed to create pipe for capturing output: %w", pipeErr)
	}
	os.Stdout = w

	var execErr error
	// Execute the external pinniped command.
	// pinnipedcmd.Execute() here refers to the Execute function from the imported "go.pinniped.dev/cmd/pinniped/cmd" package.
	execErr = pinnipedcmd.Execute()

	if err := w.Close(); err != nil && execErr == nil {
		execErr = fmt.Errorf("failed to close pipe writer: %w", err)
	}
	os.Stdout = oldStdout

	var outputBuf bytes.Buffer
	_, copyErr := io.Copy(&outputBuf, r)
	if err := r.Close(); err != nil && execErr == nil { // Close the reader end of the pipe.
		execErr = fmt.Errorf("failed to close pipe reader: %w", err)
	}

	if copyErr != nil && execErr == nil {
		execErr = fmt.Errorf("failed to read command output from pipe: %w", copyErr)
	}

	if execErr != nil {
		// If there was an error during execution, still try to print what was captured.
		if outputBuf.Len() > 0 {
			_, _ = streams.ErrOut.Write(outputBuf.Bytes())
		}
		return execErr
	}

	return processKubeconfigOutput(&outputBuf, streams)
}

func NewCmdPinniped(streams genericiooptions.IOStreams) *cobra.Command {
	cmdVar := &cobra.Command{
		Use:                "pinniped",
		DisableFlagParsing: true, // Disable flag parsing to allow Pinniped to handle its own flags
		Short:              i18n.T("pinniped provides utilities for interacting with Pinniped"),
		RunE: func(_ *cobra.Command, runEArgs []string) error {
			originalGlobalOsArgs := make([]string, len(os.Args))
			copy(originalGlobalOsArgs, os.Args)
			defer func() {
				os.Args = originalGlobalOsArgs
			}()

			os.Args = append([]string{"pinniped"}, runEArgs...)

			if len(runEArgs) >= 2 && runEArgs[0] == "get" && runEArgs[1] == "kubeconfig" {
				return handleGetKubeconfigCmd(runEArgs, streams)
			}

			// Default execution for other pinniped commands.
			// pinnipedcmd.Execute() here refers to the Execute function from the imported "go.pinniped.dev/cmd/pinniped/cmd" package.
			return pinnipedcmd.Execute()
		},
	}
	return cmdVar
}

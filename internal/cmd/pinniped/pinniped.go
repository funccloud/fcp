package pinniped

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"go.pinniped.dev/cmd/pinniped/cmd"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/yaml"
)

//nolint:gochecknoinits
func init() {
	// browsers like chrome like to write to our std out which breaks our JSON ExecCredential output
	// thus we redirect the browser's std out to our std err
	browser.Stdout = os.Stderr
}

func NewCmdPinniped(streams genericiooptions.IOStreams) *cobra.Command {
	cmdVar := &cobra.Command{
		Use:                "pinniped",
		DisableFlagParsing: true, // Disable flag parsing to allow Pinniped to handle its own flags
		Short:              i18n.T("pinniped provides utilities for interacting with Pinniped"),
		RunE: func(_ *cobra.Command, runEArgs []string) error {
			// Save current os.Args and ensure it's restored upon returning.
			originalGlobalOsArgs := make([]string, len(os.Args))
			copy(originalGlobalOsArgs, os.Args)
			defer func() {
				os.Args = originalGlobalOsArgs
			}()

			// Prepare os.Args for the external Pinniped CLI.
			// The external 'cmd.Execute()' (from go.pinniped.dev/cmd/pinniped/cmd package)
			// expects os.Args[0] to be "pinniped" and subsequent elements to be its arguments.
			os.Args = append([]string{"pinniped"}, runEArgs...)

			var execErr error

			// Check if the command is "pinniped get kubeconfig"
			if len(runEArgs) >= 2 && runEArgs[0] == "get" && runEArgs[1] == "kubeconfig" {
				// Capture stdout for "get kubeconfig" into a buffer.
				oldStdout := os.Stdout
				r, w, pipeErr := os.Pipe()
				if pipeErr != nil {
					return fmt.Errorf("failed to create pipe for capturing output: %w", pipeErr)
				}
				os.Stdout = w

				// Execute the external pinniped command.
				// cmd.Execute() here refers to the Execute function from the imported "go.pinniped.dev/cmd/pinniped/cmd" package.
				execErr = cmd.Execute()

				// Close the writer end of the pipe and restore original stdout.
				if err := w.Close(); err != nil && execErr == nil {
					execErr = fmt.Errorf("failed to close pipe writer: %w", err)
				}
				os.Stdout = oldStdout

				var outputBuf bytes.Buffer
				_, copyErr := io.Copy(&outputBuf, r)
				if err := r.Close(); err != nil && execErr == nil { // Close the reader end of the pipe.
					execErr = fmt.Errorf("failed to close pipe reader: %w", err)
				}

				// If there was an error copying from the pipe and no prior execution error, report it.
				if copyErr != nil && execErr == nil {
					execErr = fmt.Errorf("failed to read command output from pipe: %w", copyErr)
				}
				// The output is now in outputBuf.
				// To maintain existing behavior (if any) of printing to stdout,
				// and because the user hasn't specified otherwise, we write the buffer's content
				// to the streams.Out provided to this fcp command.
				if outputBuf.Len() > 0 {
					cfg := clientcmdapi.Config{}
					err := yaml.Unmarshal(outputBuf.Bytes(), &cfg)
					if err != nil {
						return fmt.Errorf("failed to unmarshal kubeconfig output: %w", err)
					}
					for _, authInfo := range cfg.AuthInfos {
						if authInfo.Exec != nil {
							if authInfo.Exec.Args == nil {
								authInfo.Exec.Args = []string{}
							}
							authInfo.Exec.Args = append([]string{"pinniped"}, authInfo.Exec.Args...)
						}
					}
					// Marshal the modified cfg back to YAML
					modifiedYAML, err := yaml.Marshal(cfg)
					if err != nil {
						return fmt.Errorf("failed to marshal modified kubeconfig: %w", err)
					}
					if _, err := streams.Out.Write(modifiedYAML); err != nil && execErr == nil {
						execErr = fmt.Errorf("failed to write output to stream: %w", err)
					}
				}
			} else {
				// Default execution for other pinniped commands.
				// cmd.Execute() here refers to the Execute function from the imported "go.pinniped.dev/cmd/pinniped/cmd" package.
				execErr = cmd.Execute()
			}

			return execErr
		},
	}
	return cmdVar
}

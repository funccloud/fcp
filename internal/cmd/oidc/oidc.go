package oidc

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

var (
	oidcLong = templates.LongDesc(i18n.T(`
		OIDC authentication commands.

		These commands provide OpenID Connect (OIDC) authentication capabilities
		for Kubernetes clusters managed by FCP.`))

	oidcExample = templates.Examples(i18n.T(`
		# Perform OIDC login
		fcp oidc login

		# Login with a specific issuer URL
		fcp oidc login --issuer-url https://accounts.google.com

		# Login with custom client ID
		fcp oidc login --client-id my-client-id`))
)

// NewCmdOIDC creates the oidc command
func NewCmdOIDC(f cmdutil.Factory, ioStreams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "oidc",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("OIDC authentication commands"),
		Long:                  oidcLong,
		Example:               oidcExample,
		Run:                   cmdutil.DefaultSubCommandRun(ioStreams.ErrOut),
	}

	cmd.AddCommand(NewCmdOIDCLogin(f, ioStreams))

	return cmd
}

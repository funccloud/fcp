package install

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.funccloud.dev/fcp/internal/resource"
	"go.funccloud.dev/fcp/internal/scheme"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Options struct {
	Domain string
	genericiooptions.IOStreams
	Client client.Client
}

func NewCmdInstall(f cmdutil.Factory, ioStreams genericiooptions.IOStreams) *cobra.Command {
	o := &Options{
		IOStreams: ioStreams,
	}
	cmd := &cobra.Command{
		Use:   "install",
		Short: i18n.T("Install the FCP components"),
		Long:  i18n.T("Install the FCP components in the current context."),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().StringVar(&o.Domain, "domain", "", "Domain for FCP")
	return cmd
}

func (o *Options) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	cfg, err := f.ToRESTConfig()
	if err != nil {
		return err
	}
	o.Client, err = client.New(cfg, client.Options{
		Scheme: scheme.Get(),
	})
	if err != nil {
		return err
	}
	return nil
}

func (o *Options) Validate() error {
	if o.Domain == "" {
		return fmt.Errorf("domain flag is required")
	}
	return nil
}

func (o *Options) Run(ctx context.Context) error {
	_, _ = fmt.Fprintf(o.Out, "Installing FCP components with domain %s\n", o.Domain)
	err := resource.CheckOrInstallVersion(ctx, o.Domain, o.Client, o.IOStreams)
	if err != nil {
		_, _ = fmt.Fprintf(o.ErrOut, "Error installing FCP components: %v\n", err)
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "FCP components installed successfully\n")
	return nil
}

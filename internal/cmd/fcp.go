package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"go.funccloud.dev/fcp/internal/cmd/install"
	"go.funccloud.dev/fcp/internal/cmd/plugin"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/cmd/version"
	"k8s.io/kubectl/pkg/kuberc"
	utilcomp "k8s.io/kubectl/pkg/util/completion"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/kubectl/pkg/util/term"
)

const fcpCmdHeaders = "FCP_COMMAND_HEADERS"

type FCPOptions struct {
	PluginHandler PluginHandler
	Arguments     []string
	ConfigFlags   *genericclioptions.ConfigFlags
	genericiooptions.IOStreams
}

func defaultConfigFlags() *genericclioptions.ConfigFlags {
	return genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag().WithDiscoveryBurst(300).WithDiscoveryQPS(50.0)
}

func NewDefaultFCPCommand() *cobra.Command {
	ioStreams := genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	return NewDefaultFCPCommandWithArgs(FCPOptions{
		PluginHandler: NewDefaultPluginHandler(plugin.ValidPluginFilenamePrefixes, plugin.ValidSubcommandBinaries),
		Arguments:     os.Args,
		ConfigFlags:   defaultConfigFlags().WithWarningPrinter(ioStreams),
		IOStreams:     ioStreams,
	})
}

func NewDefaultFCPCommandWithArgs(o FCPOptions) *cobra.Command {
	cmd := NewFCPCommand(o)

	if o.PluginHandler == nil {
		return cmd
	}

	if len(o.Arguments) > 1 {
		cmdPathPieces := o.Arguments[1:]

		// only look for suitable extension executables if
		// the specified command does not already exist
		foundCmd, foundArgs, err := cmd.Find(cmdPathPieces)
		if err != nil {
			// Also check the commands that will be added by Cobra.
			// These commands are only added once rootCmd.Execute() is called, so we
			// need to check them explicitly here.
			var cmdName string // first "non-flag" arguments
			for _, arg := range cmdPathPieces {
				if !strings.HasPrefix(arg, "-") {
					cmdName = arg
					break
				}
			}

			switch cmdName {
			case "help", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
				// Don't search for a plugin
			default:
				if err := HandlePluginCommand(o.PluginHandler, cmdPathPieces, 1); err != nil {
					fmt.Fprintf(o.IOStreams.ErrOut, "Error: %v\n", err) // nolint:errcheck
					os.Exit(1)
				}
			}
		}
		// Command exists(e.g. kubectl create), but it is not certain that
		// subcommand also exists (e.g. kubectl create networkpolicy)
		// we also have to eliminate kubectl create -f
		if IsSubcommandPluginAllowed(foundCmd.Name()) && len(foundArgs) >= 1 && !strings.HasPrefix(foundArgs[0], "-") {
			subcommand := foundArgs[0]
			builtinSubcmdExist := false
			for _, subcmd := range foundCmd.Commands() {
				if subcmd.Name() == subcommand {
					builtinSubcmdExist = true
					break
				}
			}

			if !builtinSubcmdExist {
				if err := HandlePluginCommand(o.PluginHandler, cmdPathPieces, len(cmdPathPieces)-len(foundArgs)+1); err != nil {
					fmt.Fprintf(o.IOStreams.ErrOut, "Error: %v\n", err) // nolint:errcheck
					os.Exit(1)
				}
			}
		}
	}

	return cmd
}

// IsSubcommandPluginAllowed returns the given command is allowed
// to use plugin as subcommand if the subcommand does not exist as builtin.
func IsSubcommandPluginAllowed(foundCmd string) bool {
	allowedCmds := map[string]struct{}{"create": {}}
	_, ok := allowedCmds[foundCmd]
	return ok
}

// GetLogVerbosity returns the verbosity level for the command line arguments.
func GetLogVerbosity(args []string) string {
	for i, arg := range args {
		if arg == "--" {
			// flags after "--" does not represent any flag of
			// the command. We should short cut the iteration in here.
			break
		}

		if arg == "--v" || arg == "-v" {
			if i+1 < len(args) {
				return args[i+1]
			}
		} else if strings.Contains(arg, "--v=") || strings.Contains(arg, "-v=") {
			parg := strings.Split(arg, "=")
			if len(parg) > 1 && parg[1] != "" {
				return parg[1]
			}
		}
	}

	return "0"
}

func shouldSkipOnLookPathErr(err error) bool {
	return err != nil && !errors.Is(err, exec.ErrDot)
}

// PluginHandler is capable of parsing command line arguments
// and performing executable filename lookups to search
// for valid plugin files, and execute found plugins.
type PluginHandler interface {
	// exists at the given filename, or a boolean false.
	// Lookup will iterate over a list of given prefixes
	// in order to recognize valid plugin filenames.
	// The first filepath to match a prefix is returned.
	Lookup(filename string) (string, bool)
	// Execute receives an executable's filepath, a slice
	// of arguments, and a slice of environment variables
	// to relay to the executable.
	Execute(executablePath string, cmdArgs, environment []string) error
}

// DefaultPluginHandler implements PluginHandler
type DefaultPluginHandler struct {
	ValidPrefixes      []string
	SubCommandBinaries map[string]string
}

// NewDefaultPluginHandler instantiates the DefaultPluginHandler with a list of
// given filename prefixes used to identify valid plugin filenames.
func NewDefaultPluginHandler(validPrefixes []string, subCommandBinaries map[string]string) *DefaultPluginHandler {
	return &DefaultPluginHandler{
		ValidPrefixes:      validPrefixes,
		SubCommandBinaries: subCommandBinaries,
	}
}

// Lookup implements PluginHandler
func (h *DefaultPluginHandler) Lookup(filename string) (string, bool) {
	name, ok := plugin.ValidSubcommandBinaries[filename]
	if !ok {
		_, ok = plugin.ThirdPartyPlugin(filename)
		if ok {
			name = filename
		}
	}
	if ok {
		path, err := exec.LookPath(name)
		if shouldSkipOnLookPathErr(err) || len(path) == 0 {
			return "", false
		}
		return path, true
	}
	for _, prefix := range h.ValidPrefixes {
		path, err := exec.LookPath(fmt.Sprintf("%s-%s", prefix, filename))
		if shouldSkipOnLookPathErr(err) || len(path) == 0 {
			continue
		}
		return path, true
	}
	return "", false
}

func Command(name string, arg ...string) *exec.Cmd {
	cmd := &exec.Cmd{
		Path: name,
		Args: append([]string{name}, arg...),
	}
	if filepath.Base(name) == name {
		lp, err := exec.LookPath(name)
		if lp != "" && !shouldSkipOnLookPathErr(err) {
			// Update cmd.Path even if err is non-nil.
			// If err is ErrDot (especially on Windows), lp may include a resolved
			// extension (like .exe or .bat) that should be preserved.
			cmd.Path = lp
		}
	}
	return cmd
}

// Execute implements PluginHandler
func (h *DefaultPluginHandler) Execute(executablePath string, cmdArgs, environment []string) error {

	// Windows does not support exec syscall.
	if runtime.GOOS == "windows" {
		cmd := Command(executablePath, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = environment
		err := cmd.Run()
		if err == nil {
			os.Exit(0)
		}
		return err
	}

	// invoke cmd binary relaying the environment and args given
	// append executablePath to cmdArgs, as execve will make first argument the "binary name".
	return syscall.Exec(executablePath, append([]string{executablePath}, cmdArgs...), environment)
}

// HandlePluginCommand receives a pluginHandler and command-line arguments and attempts to find
// a plugin executable on the PATH that satisfies the given arguments.
func HandlePluginCommand(pluginHandler PluginHandler, cmdArgs []string, minArgs int) error {
	var remainingArgs []string // nolint:prealloc
	for i, arg := range cmdArgs {
		if strings.HasPrefix(arg, "-") {
			break
		}
		t, ok := plugin.ValidSubcommandBinaries[arg]
		if i == 0 && ok {
			arg = t
		}
		remainingArgs = append(remainingArgs, strings.Replace(arg, "-", "_", -1))
	}

	if len(remainingArgs) == 0 {
		// the length of cmdArgs is at least 1
		return fmt.Errorf("flags cannot be placed before plugin name: %s", cmdArgs[0])
	}

	foundBinaryPath := ""
	// attempt to find binary, starting at longest possible name with given cmdArgs
	for len(remainingArgs) > 0 {
		path, found := pluginHandler.Lookup(strings.Join(remainingArgs, "-"))
		if !found {
			remainingArgs = remainingArgs[:len(remainingArgs)-1]
			if len(remainingArgs) < minArgs {
				// we shouldn't continue searching with shorter names.
				// this is especially for not searching fcp-create plugin
				// when fcp-create-foo plugin is not found.
				break
			}

			continue
		}

		foundBinaryPath = path
		break
	}

	if len(foundBinaryPath) == 0 {
		return nil
	}
	// invoke cmd binary relaying the current environment and args given
	if err := pluginHandler.Execute(foundBinaryPath, cmdArgs[len(remainingArgs):], os.Environ()); err != nil {
		return err
	}

	return nil
}

func NewFCPCommand(o FCPOptions) *cobra.Command {
	warningHandler := rest.NewWarningWriter(o.IOStreams.ErrOut, rest.WarningWriterOptions{Deduplicate: true, Color: term.AllowsColorOutput(o.IOStreams.ErrOut)})
	warningsAsErrors := false
	// Parent command to which all subcommands are added.
	cmds := &cobra.Command{
		Use:   "fcp",
		Short: i18n.T("fcp controls the FuncCloud Platform"),
		Long: templates.LongDesc(`
      fcp controls the FuncCloud Platform`),
		Run: runHelp,
		// Hook before and after Run initialize and write profiles to disk,
		// respectively.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			plugin.SetDirEnv()
			rest.SetDefaultWarningHandler(warningHandler)

			if cmd.Name() == cobra.ShellCompRequestCmd {
				// This is the __complete or __completeNoDesc command which
				// indicates shell completion has been requested.
				plugin.SetupPluginCompletion(cmd, args, o.IOStreams)
			}

			return initProfiling()
		},
		PersistentPostRunE: func(*cobra.Command, []string) error {
			if err := flushProfiling(); err != nil {
				return err
			}
			if warningsAsErrors {
				count := warningHandler.WarningCount()
				switch count {
				case 0:
					// no warnings
				case 1:
					return fmt.Errorf("%d warning received", count)
				default:
					return fmt.Errorf("%d warnings received", count)
				}
			}
			return nil
		},
	}
	// From this point and forward we get warnings on flags that contain "_" separators
	// when adding them with hyphen instead of the original name.
	cmds.SetGlobalNormalizationFunc(cliflag.WarnWordSepNormalizeFunc)

	flags := cmds.PersistentFlags()

	addProfilingFlags(flags)

	flags.BoolVar(&warningsAsErrors, "warnings-as-errors", warningsAsErrors, "Treat warnings received from the server as errors and exit with a non-zero exit code")

	pref := kuberc.NewPreferences()
	if cmdutil.KubeRC.IsEnabled() {
		pref.AddFlags(flags)
	}

	kubeConfigFlags := o.ConfigFlags
	if kubeConfigFlags == nil {
		kubeConfigFlags = defaultConfigFlags().WithWarningPrinter(o.IOStreams)
	}
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(flags)
	// Updates hooks to add fcp command headers: SIG CLI KEP 859.
	addCmdHeaderHooks(cmds, kubeConfigFlags)

	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)

	groups := templates.CommandGroups{}
	groups.Add(cmds)

	filters := []string{"options"}

	// Add plugin command group to the list of command groups.
	// The commands are only injected for the scope of showing help and completion, they are not
	// invoked directly.
	pluginCommandGroup := plugin.GetPluginCommandGroup(cmds, o.IOStreams)
	groups = append(groups, pluginCommandGroup)

	templates.ActsAsRootCommand(cmds, filters, groups...)

	utilcomp.SetFactoryForCompletion(f)
	registerCompletionFuncForGlobalFlags(cmds, f)

	cmds.AddCommand(plugin.NewCmdPlugin(o.IOStreams))
	cmds.AddCommand(version.NewCmdVersion(f, o.IOStreams))
	cmds.AddCommand(install.NewCmdInstall(f, o.IOStreams))

	// Stop warning about normalization of flags. That makes it possible to
	// add the klog flags later.
	cmds.SetGlobalNormalizationFunc(cliflag.WordSepNormalizeFunc)

	if cmdutil.KubeRC.IsEnabled() {
		_, err := pref.Apply(cmds, o.Arguments, o.IOStreams.ErrOut)
		if err != nil {
			fmt.Fprintf(o.IOStreams.ErrOut, "error occurred while applying preferences %v\n", err) // nolint:errcheck
			os.Exit(1)
		}
	}

	return cmds
}

func addCmdHeaderHooks(cmds *cobra.Command, kubeConfigFlags *genericclioptions.ConfigFlags) {
	// If the feature gate env var is set to "false", then do no add fcp command headers.
	if value, exists := os.LookupEnv(fcpCmdHeaders); exists {
		if value == "false" || value == "0" {
			klog.V(5).Infoln("fcp command headers turned off")
			return
		}
	}
	klog.V(5).Infoln("fcp command headers turned on")
	crt := &genericclioptions.CommandHeaderRoundTripper{}
	existingPreRunE := cmds.PersistentPreRunE
	// Add command parsing to the existing persistent pre-run function.
	cmds.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		crt.ParseCommandHeaders(cmd, args)
		return existingPreRunE(cmd, args)
	}
	wrapConfigFn := kubeConfigFlags.WrapConfigFn
	// Wraps CommandHeaderRoundTripper around standard RoundTripper.
	kubeConfigFlags.WrapConfigFn = func(c *rest.Config) *rest.Config {
		if wrapConfigFn != nil {
			c = wrapConfigFn(c)
		}
		c.Wrap(func(rt http.RoundTripper) http.RoundTripper {
			// Must be separate RoundTripper; not "crt" closure.
			// Fixes: https://github.com/kubernetes/fcp/issues/1098
			return &genericclioptions.CommandHeaderRoundTripper{
				Delegate: rt,
				Headers:  crt.Headers,
			}
		})
		return c
	}
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help() // nolint:errcheck
}

func registerCompletionFuncForGlobalFlags(cmd *cobra.Command, f cmdutil.Factory) {
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"namespace",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.CompGetResource(f, "namespace", toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"context",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListContextsInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"cluster",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListClustersInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"user",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListUsersInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
}

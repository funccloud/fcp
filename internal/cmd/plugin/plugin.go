package plugin

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"go.funccloud.dev/fcp/internal/config"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

var (
	pluginLong = templates.LongDesc(i18n.T(`
		Provides utilities for interacting with plugins.

		Plugins provide extended functionality that is not part of the major command-line distribution.
		Please refer to the documentation and examples for more information about how write your own plugins.

		The easiest way to discover and install plugins is via the kubernetes sub-project krew: [krew.sigs.k8s.io].
		To install krew, visit https://krew.sigs.k8s.io/docs/user-guide/setup/install`))

	pluginExample = templates.Examples(i18n.T(`
		# List all available plugins
		kubectl plugin list
		
		# List only binary names of available plugins without paths
		kubectl plugin list --name-only`))

	pluginListLong = templates.LongDesc(i18n.T(`
		List all available plugin files on a user's PATH or in plugins directory $HOME/.fcp/pluguins.
		To see plugins binary names without the full path use --name-only flag.

		Available plugin files are those that are:
		- executable
		- anywhere on the user's PATH or in plugins directory $HOME/.fcp/plugins
		- begin with "fcp-"
`))

	ValidPluginFilenamePrefixes = []string{"fcp"}

	ValidSubcommandBinaries = map[string]string{
		"helm": "helm",
	}
)

func GetDir() string {
	return filepath.Join(config.GetConfigDir(), "plugins")
}

func SetDirEnv() {
	// Set the plugin directory to $HOME/.fcp/plugins
	pluginDir := GetDir()
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create plugin directory %q: %v\n", pluginDir, err) // nolint:errcheck
	}
	pathEnv := os.Getenv("PATH")
	if !strings.Contains(pathEnv, pluginDir) {
		err := os.Setenv("PATH", fmt.Sprintf("%s:%s", pathEnv, pluginDir))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to set plugin directory %q in PATH: %v\n", pluginDir, err) // nolint:errcheck
		}
	}
}

func NewCmdPlugin(streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "plugin [flags]",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Provides utilities for interacting with plugins"),
		Long:                  pluginLong,
		Example:               pluginExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.DefaultSubCommandRun(streams.ErrOut)(cmd, args)
		},
	}

	cmd.AddCommand(NewCmdPluginList(streams))
	return cmd
}

type PluginListOptions struct {
	Verifier    PathVerifier
	PluginPaths []string

	genericiooptions.IOStreams
}

// NewCmdPluginList provides a way to list all plugin executables visible to kubectl
func NewCmdPluginList(streams genericiooptions.IOStreams) *cobra.Command {
	o := &PluginListOptions{
		IOStreams: streams,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   i18n.T("List all visible plugin executables on a user's PATH"),
		Example: pluginExample,
		Long:    pluginListLong,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(cmd))
			cmdutil.CheckErr(o.Run())
		},
	}
	return cmd
}

func (o *PluginListOptions) Complete(cmd *cobra.Command) error {
	o.Verifier = &CommandOverrideVerifier{
		root:        cmd.Root(),
		seenPlugins: make(map[string]string),
	}
	SetDirEnv()
	o.PluginPaths = filepath.SplitList(os.Getenv("PATH"))
	return nil
}

func (o *PluginListOptions) Run() error {
	plugins, pluginErrors := o.ListPlugins()

	if len(plugins) > 0 {
		fmt.Fprintf(o.Out, "The following compatible plugins are available:\n\n") // nolint:errcheck
	} else {
		pluginErrors = append(pluginErrors, fmt.Errorf("error: unable to find any kubectl plugins in your PATH"))
	}

	pluginWarnings := 0
	for _, pluginPath := range plugins {
		base := filepath.Base(pluginPath)
		name, ok := ThirdPartyPlugin(base)
		if ok {
			base = name
		}
		fmt.Fprintf(o.Out, "%s\n", base) // nolint:errcheck
		if errs := o.Verifier.Verify(pluginPath); len(errs) != 0 {
			for _, err := range errs {
				fmt.Fprintf(o.ErrOut, "  - %s\n", err) // nolint:errcheck
				pluginWarnings++
			}
		}
	}

	if pluginWarnings > 0 {
		if pluginWarnings == 1 {
			pluginErrors = append(pluginErrors, fmt.Errorf("error: one plugin warning was found"))
		} else {
			pluginErrors = append(pluginErrors, fmt.Errorf("error: %v plugin warnings were found", pluginWarnings))
		}
	}
	if len(pluginErrors) > 0 {
		errs := bytes.NewBuffer(nil)
		for _, e := range pluginErrors {
			_, _ = fmt.Fprintln(errs, e)
		}
		return fmt.Errorf("%s", errs.String())
	}

	return nil
}

// ListPlugins returns list of plugin paths.
func (o *PluginListOptions) ListPlugins() ([]string, []error) {
	plugins := []string{}
	errors := []error{}
	for _, dir := range uniquePathsList(o.PluginPaths) {
		if len(strings.TrimSpace(dir)) == 0 {
			continue
		}

		files, err := os.ReadDir(dir)
		if err != nil {
			if _, ok := err.(*os.PathError); ok {
				fmt.Fprintf(o.ErrOut, "Unable to read directory %q from your PATH: %v. Skipping...\n", dir, err) // nolint:errcheck
				continue
			}

			errors = append(errors, fmt.Errorf("error: unable to read directory %q in your PATH: %v", dir, err))
			continue
		}

		for _, f := range files {
			if f.IsDir() {
				continue
			}
			if !hasValidPrefix(f.Name(), ValidPluginFilenamePrefixes, ValidSubcommandBinaries) {
				continue
			}

			plugins = append(plugins, filepath.Join(dir, f.Name()))
		}
	}

	return plugins, errors
}

// pathVerifier receives a path and determines if it is valid or not
type PathVerifier interface {
	// Verify determines if a given path is valid
	Verify(path string) []error
}

type CommandOverrideVerifier struct {
	root        *cobra.Command
	seenPlugins map[string]string
}

// Verify implements PathVerifier and determines if a given path
// is valid depending on whether or not it overwrites an existing
// kubectl command path, or a previously seen plugin.
func (v *CommandOverrideVerifier) Verify(path string) []error {
	if v.root == nil {
		return []error{fmt.Errorf("unable to verify path with nil root")}
	}

	// extract the plugin binary name
	segs := strings.Split(path, "/")
	binName := segs[len(segs)-1]

	cmdPath := strings.Split(binName, "-")
	if len(cmdPath) > 1 {
		// the first argument is always "kubectl" for a plugin binary
		cmdPath = cmdPath[1:]
	}

	errors := []error{}

	if isExec, err := isExecutable(path); err == nil && !isExec {
		errors = append(errors, fmt.Errorf("warning: %s identified as a kubectl plugin, but it is not executable", path))
	} else if err != nil {
		errors = append(errors, fmt.Errorf("error: unable to identify %s as an executable file: %v", path, err))
	}

	if existingPath, ok := v.seenPlugins[binName]; ok {
		errors = append(errors, fmt.Errorf("warning: %s is overshadowed by a similarly named plugin: %s", path, existingPath))
	} else {
		v.seenPlugins[binName] = path
	}

	if cmd, _, err := v.root.Find(cmdPath); err == nil {
		errors = append(errors, fmt.Errorf("warning: %s overwrites existing command: %q", binName, cmd.CommandPath()))
	}

	return errors
}

func isExecutable(fullPath string) (bool, error) {
	info, err := os.Stat(fullPath)
	if err != nil {
		return false, err
	}

	if runtime.GOOS == "windows" {
		fileExt := strings.ToLower(filepath.Ext(fullPath))

		switch fileExt {
		case ".bat", ".cmd", ".com", ".exe", ".ps1":
			return true, nil
		}
		return false, nil
	}

	if m := info.Mode(); !m.IsDir() && m&0111 != 0 {
		return true, nil
	}

	return false, nil
}

// uniquePathsList deduplicates a given slice of strings without
// sorting or otherwise altering its order in any way.
func uniquePathsList(paths []string) []string {
	return sets.NewString(paths...).UnsortedList()
}

func hasValidPrefix(filepath string, validPrefixes []string, binariesCommandMap map[string]string) bool {
	for _, bin := range binariesCommandMap {
		if filepath == bin {
			return true // this is a valid plugin binary, so we don't need to check the prefix
		}
	}
	for _, prefix := range validPrefixes {
		if !strings.HasPrefix(filepath, prefix+"-") {
			continue
		}
		return true
	}
	return false
}

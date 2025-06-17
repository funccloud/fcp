package kong

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

const (
	kongRepoURL     = "https://charts.konghq.com"
	kongChartName   = "kong"
	kngChartVersion = "2.49.0"
)

func CheckOrInstallVersion(ctx context.Context, pluginsDir string, isKind bool, ioStreams genericiooptions.IOStreams) error {
	helmBin := filepath.Join(pluginsDir, "helm")
	installed, err := isKongInstalled(ctx, helmBin, ioStreams)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error checking Kong installation", "error", err)
		return err
	}
	if !installed {
		if err := helmAddRepo(ctx, helmBin, kongChartName, kongRepoURL, ioStreams); err != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error adding Kong Helm repository", "error", err)
			return err
		}
		if err := helmUpdateRepo(ctx, helmBin, ioStreams); err != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error updating Kong Helm repository", "error", err)
			return err
		}
		if err := helmUpgradeOrInstallKong(ctx, helmBin, kongChartName, isKind, ioStreams); err != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error upgrading or installing Kong", "error", err)
			return err
		}
		_, _ = fmt.Fprintln(ioStreams.Out, "Kong installed successfully")
	}
	return nil
}

func helmAddRepo(ctx context.Context, helmBin, repoName, repoURL string, ioStreams genericiooptions.IOStreams) error {
	cmd := exec.CommandContext(ctx, helmBin, "repo", "add", repoName, repoURL)
	cmd.Stdout = ioStreams.Out
	cmd.Stderr = ioStreams.ErrOut
	cmd.Stdin = ioStreams.In
	return cmd.Run()
}

func helmUpdateRepo(ctx context.Context, helmBin string, ioStreams genericiooptions.IOStreams) error {
	cmd := exec.CommandContext(ctx, helmBin, "repo", "update")
	cmd.Stdout = ioStreams.Out
	cmd.Stderr = ioStreams.ErrOut
	cmd.Stdin = ioStreams.In
	return cmd.Run()
}

func isKongInstalled(ctx context.Context, helmBin string, ioStreams genericiooptions.IOStreams) (bool, error) {
	cmd := exec.CommandContext(ctx, helmBin, "list", "--all", "--filter", kongChartName, "--output", "json")
	cmd.Stdout = ioStreams.Out
	cmd.Stderr = ioStreams.ErrOut
	cmd.Stdin = ioStreams.In
	output, err := cmd.Output()
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error checking Kong installation", "error", err)
		return false, err
	}
	type helmRelease struct {
		Name       string `json:"name"`
		Chart      string `json:"chart"`
		AppVersion string `json:"app_version"`
	}
	var releases []helmRelease
	err = json.Unmarshal(output, &releases)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error parsing helm list output", "error", err)
		return false, err
	}
	for _, rel := range releases {
		if rel.Name == kongChartName {
			// Chart field is like "kong-2.49.0"
			if strings.HasSuffix(rel.Chart, kngChartVersion) {
				return true, nil
			}
			return false, nil
		}
	}
	return false, nil
}

func helmUpgradeOrInstallKong(ctx context.Context, helmBin, namespace string, isKind bool, ioStreams genericiooptions.IOStreams) error {
	args := []string{
		"upgrade", "--install", kongChartName, fmt.Sprintf("%s/%s", kongChartName, kongChartName),
		"--namespace", namespace,
		"--create-namespace",
		"--version", kngChartVersion,
	}
	if isKind {
		args = append(args,
			"--set", "proxy.type=NodePort",
			"--set", "proxy.http.nodePort=31080",
			"--set", "proxy.https.nodePort=31443",
		)
	}
	cmd := exec.CommandContext(ctx, helmBin, args...)
	cmd.Stdout = ioStreams.Out
	cmd.Stderr = ioStreams.ErrOut
	cmd.Stdin = ioStreams.In
	return cmd.Run()
}

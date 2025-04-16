{ pkgs, ... }:
{
  env = {
    # Set up environment variables if needed
  };
  packages = [
    pkgs.go_1_24
    pkgs.docker
    pkgs.git
    pkgs.zsh
    pkgs.curl
    pkgs.bash
    pkgs.gnugrep
    pkgs.gnused
    pkgs.coreutils
  ];
  idx.extensions = [
    "ms-kubernetes-tools.vscode-kubernetes-tools"
    "ms-azuretools.vscode-docker"
  ];
  idx.prebuild = {
    shell = "bash post-install.sh";
  };
}

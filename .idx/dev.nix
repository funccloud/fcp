{ pkgs, ... }:
{
  env = {
    # Set up environment variables if needed
  };
  packages = [
    pkgs.go
    pkgs.docker
    pkgs.git
    pkgs.zsh
    pkgs.curl
    pkgs.bash
    pkgs.gnugrep
    pkgs.gnused
    pkgs.coreutils
    pkgs.tilt
  ];
  idx.workspace = {
    onCreate = {
      post-install = "bash .idx/post-install.sh";
    };
  };
  services.docker.enable = true;
  idx.extensions = [
    "ms-kubernetes-tools.vscode-kubernetes-tools"
    "ms-azuretools.vscode-docker"
    "redhat.vscode-yaml"
  ];
}

#!/bin/bash
set -x

# Check if Docker daemon is running
if ! docker info &>/dev/null; then
  echo "Error: Docker daemon is not running. Please start Docker."
  exit 1
fi



BIN_DIR="$PWD/bin"
# Create a bin directory within the project if it doesn't exist
mkdir -p bin

# Download kind binary
curl -Lo ./bin/kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64
chmod +x ./bin/kind

# Download kubebuilder binary
curl -L -o bin/kubebuilder https://go.kubebuilder.io/dl/latest/linux/amd64
chmod +x bin/kubebuilder

# Download kubectl binary
KUBECTL_VERSION=$(curl -L -s https://dl.k8s.io/release/stable.txt)
curl -Lo ./bin/kubectl "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/linux/amd64/kubectl"
chmod +x ./bin/kubectl


# Update the PATH for the current session
export PATH="$BIN_DIR:$PATH"

if ! docker network inspect kind &>/dev/null; then
  docker network create -d=bridge --subnet=172.19.0.0/24 kind
fi

kind version
kubebuilder version
docker --version
go version
kubectl version --client

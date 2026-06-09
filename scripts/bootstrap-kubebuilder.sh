#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE_PATH="github.com/linshanyu/astra-scheduler"
DOMAIN="aiinfra.io"
GROUP="astra"
VERSION="v1alpha1"

cd "${PROJECT_ROOT}"

echo "[astra] project root: ${PROJECT_ROOT}"

if ! command -v go >/dev/null 2>&1; then
  echo "[astra] go command not found. Please install Go first." >&2
  exit 1
fi

if ! command -v kubebuilder >/dev/null 2>&1; then
  echo "[astra] kubebuilder command not found. Please add it to PATH first." >&2
  exit 1
fi

if [ ! -f go.mod ]; then
  echo "[astra] initializing go module: ${MODULE_PATH}"
  go mod init "${MODULE_PATH}"
else
  echo "[astra] go.mod already exists, skip go mod init"
fi

if [ ! -f PROJECT ]; then
  echo "[astra] initializing kubebuilder project"
  kubebuilder init \
    --domain "${DOMAIN}" \
    --repo "${MODULE_PATH}"
else
  echo "[astra] PROJECT already exists, skip kubebuilder init"
fi

create_api_if_missing() {
  local kind="$1"
  local type_file="api/${VERSION}/$(echo "${kind}" | tr '[:upper:]' '[:lower:]')_types.go"

  if [ -f "${type_file}" ]; then
    echo "[astra] ${kind} API already exists, skip"
    return
  fi

  echo "[astra] creating API resource ${kind} without controller"
  kubebuilder create api \
    --group "${GROUP}" \
    --version "${VERSION}" \
    --kind "${kind}" \
    --resource=true \
    --controller=false
}

create_api_if_missing "AIWorkloadProfile"
create_api_if_missing "AINodeResourceProfile"
create_api_if_missing "AIResourceAllocation"

echo "[astra] formatting and downloading dependencies"
go fmt ./...
go mod tidy

echo "[astra] generating deepcopy code and CRD manifests"
make generate
make manifests

echo "[astra] done"
echo "[astra] next files to edit:"
echo "  api/${VERSION}/aiworkloadprofile_types.go"
echo "  api/${VERSION}/ainoderesourceprofile_types.go"
echo "  api/${VERSION}/airesourceallocation_types.go"

#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCENARIO="${1:-}"

if [[ -z "${SCENARIO}" ]]; then
  echo "usage: $0 <scenario-name>"
  echo
  echo "available scenarios:"
  find "${ROOT_DIR}/evaluation/scenarios" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort
  exit 1
fi

MANIFEST="${ROOT_DIR}/evaluation/scenarios/${SCENARIO}/manifests.yaml"

if [[ ! -f "${MANIFEST}" ]]; then
  echo "scenario manifest not found: ${MANIFEST}" >&2
  exit 1
fi

echo "[astra-eval] deleting scenario: ${SCENARIO}"
kubectl delete -f "${MANIFEST}" --ignore-not-found=true

echo "[astra-eval] deleting leftover allocations in astra-eval"
kubectl delete airesourceallocations -n astra-eval --all --ignore-not-found=true

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

SCENARIO_DIR="${ROOT_DIR}/evaluation/scenarios/${SCENARIO}"
MANIFEST="${SCENARIO_DIR}/manifests.yaml"

if [[ ! -f "${MANIFEST}" ]]; then
  echo "scenario manifest not found: ${MANIFEST}" >&2
  exit 1
fi

echo "[astra-eval] applying base fake cluster"
kubectl apply -f "${ROOT_DIR}/evaluation/base/fake-cluster.yaml"

echo "[astra-eval] applying scenario: ${SCENARIO}"
kubectl apply -f "${MANIFEST}"

echo
echo "[astra-eval] wait a few seconds, then inspect:"
echo "  kubectl get pods -n astra-eval -o wide"
echo "  kubectl get airesourceallocations -n astra-eval -o wide"
echo "  kubectl get ainoderesourceprofiles -n astra-scheduler-system"

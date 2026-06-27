#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULT_ROOT="${ROOT_DIR}/evaluation/results/$(date +%Y%m%d-%H%M%S)"
WAIT_SECONDS="${WAIT_SECONDS:-25}"
SCENARIOS=("${@}")

if [[ ${#SCENARIOS[@]} -eq 0 ]]; then
  mapfile -t SCENARIOS < <(find "${ROOT_DIR}/evaluation/scenarios" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort)
fi

mkdir -p "${RESULT_ROOT}"

echo "[astra-eval] result root: ${RESULT_ROOT}"
echo "[astra-eval] applying base fake cluster"
kubectl apply -f "${ROOT_DIR}/evaluation/base/fake-cluster.yaml" | tee "${RESULT_ROOT}/base-apply.log"

collect_common() {
  local out_dir="$1"

  kubectl get nodes -o wide >"${out_dir}/nodes.txt" 2>&1 || true
  kubectl get pods -n astra-eval -o wide >"${out_dir}/pods-wide.txt" 2>&1 || true
  kubectl get pods -n astra-eval -o yaml >"${out_dir}/pods.yaml" 2>&1 || true
  kubectl get airesourceallocations -A -o wide >"${out_dir}/allocations-wide.txt" 2>&1 || true
  kubectl get airesourceallocations -A -o yaml >"${out_dir}/allocations.yaml" 2>&1 || true
  kubectl get ainoderesourceprofiles -n astra-scheduler-system -o wide >"${out_dir}/nodeprofiles-wide.txt" 2>&1 || true
  kubectl get ainoderesourceprofiles -n astra-scheduler-system -o yaml >"${out_dir}/nodeprofiles.yaml" 2>&1 || true
  kubectl get events -n astra-eval --sort-by=.lastTimestamp >"${out_dir}/events.txt" 2>&1 || true
  systemctl status astra-scheduler.service --no-pager >"${out_dir}/astra-scheduler-status.txt" 2>&1 || true
  journalctl -u astra-scheduler.service -n 200 --no-pager >"${out_dir}/astra-scheduler.log" 2>&1 || true
  local local_node
  local_node="$(hostname)"
  systemctl status "astra-agent@${local_node}.service" --no-pager >"${out_dir}/astra-agent-${local_node}-status.txt" 2>&1 || true
  journalctl -u "astra-agent@${local_node}.service" -n 200 --no-pager >"${out_dir}/astra-agent-${local_node}.log" 2>&1 || true
  printf '%s
' "Only the local node agent log is collected. Remote agent health is reflected by allocation phase transitions." >"${out_dir}/agent-log-scope.txt"
}

for scenario in "${SCENARIOS[@]}"; do
  scenario_dir="${ROOT_DIR}/evaluation/scenarios/${scenario}"
  manifest="${scenario_dir}/manifests.yaml"
  out_dir="${RESULT_ROOT}/${scenario}"

  if [[ ! -f "${manifest}" ]]; then
    echo "[astra-eval] missing scenario manifest: ${manifest}" >&2
    exit 1
  fi

  mkdir -p "${out_dir}"
  cp "${manifest}" "${out_dir}/manifests.yaml"
  [[ -f "${scenario_dir}/README.md" ]] && cp "${scenario_dir}/README.md" "${out_dir}/README.md"

  echo
  echo "[astra-eval] scenario: ${scenario}"
  echo "[astra-eval] cleanup previous scenario resources"
  : >"${out_dir}/cleanup.log"
  while IFS= read -r previous_manifest; do
    kubectl delete -f "${previous_manifest}" --ignore-not-found=true >>"${out_dir}/cleanup.log" 2>&1 || true
  done < <(find "${ROOT_DIR}/evaluation/scenarios" -mindepth 2 -maxdepth 2 \( -name manifests.yaml -o -name 'phase*.yaml' \) | sort)
  kubectl delete deployments,pods,aiworkloadprofiles -n astra-eval --all --ignore-not-found=true >>"${out_dir}/cleanup.log" 2>&1 || true
  kubectl delete airesourceallocations --all --all-namespaces --ignore-not-found=true >>"${out_dir}/cleanup.log" 2>&1 || true
  sleep 5

  echo "[astra-eval] apply ${scenario}"
  if [[ -f "${scenario_dir}/phase1-low.yaml" && -f "${scenario_dir}/phase2-peak.yaml" ]]; then
    cp "${scenario_dir}/phase1-low.yaml" "${out_dir}/phase1-low.yaml"
    cp "${scenario_dir}/phase2-peak.yaml" "${out_dir}/phase2-peak.yaml"
    {
      echo "[astra-eval] apply phase1-low"
      kubectl apply -f "${scenario_dir}/phase1-low.yaml"
      echo "[astra-eval] wait for phase1 ${WAIT_SECONDS}s"
      sleep "${WAIT_SECONDS}"
      echo "[astra-eval] apply phase2-peak"
      kubectl apply -f "${scenario_dir}/phase2-peak.yaml"
    } >"${out_dir}/apply.log" 2>&1
  else
    kubectl apply -f "${manifest}" >"${out_dir}/apply.log" 2>&1
  fi

  echo "[astra-eval] wait ${WAIT_SECONDS}s"
  sleep "${WAIT_SECONDS}"

  echo "[astra-eval] collect ${scenario}"
  collect_common "${out_dir}"

  {
    echo "# ${scenario}"
    echo
    echo "## Pods"
    cat "${out_dir}/pods-wide.txt"
    echo
    echo "## Allocations"
    cat "${out_dir}/allocations-wide.txt"
    echo
    echo "## Node Profiles"
    cat "${out_dir}/nodeprofiles-wide.txt"
  } >"${out_dir}/SUMMARY.md"

  echo "[astra-eval] wrote ${out_dir}/SUMMARY.md"
done

ln -sfn "${RESULT_ROOT}" "${ROOT_DIR}/evaluation/results/latest"
echo
echo "[astra-eval] done: ${RESULT_ROOT}"

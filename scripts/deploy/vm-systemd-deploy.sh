#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist/vm-systemd"
BIN_DIR="${DIST_DIR}/bin"
MANIFEST_DIR="${DIST_DIR}/manifests"
SERVICE_DIR="${DIST_DIR}/systemd"

MASTER0_IP="${MASTER0_IP:-192.168.64.17}"
MASTER1_IP="${MASTER1_IP:-192.168.64.18}"
WORKER0_IP="${WORKER0_IP:-192.168.64.19}"
WORKER1_IP="${WORKER1_IP:-192.168.64.20}"

MASTER0_USER="${MASTER0_USER:-master0}"
MASTER1_USER="${MASTER1_USER:-master1}"
WORKER0_USER="${WORKER0_USER:-worker0}"
WORKER1_USER="${WORKER1_USER:-worker1}"

ASTRA_NAMESPACE="${ASTRA_NAMESPACE:-astra-scheduler-system}"
WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE:-ai}"
REMOTE_ROOT="${REMOTE_ROOT:-/opt/astra-scheduler}"
REMOTE_ETC="${REMOTE_ETC:-/etc/astra-scheduler}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-/etc/kubernetes/admin.conf}"
SUDO_PASSWORD="${SUDO_PASSWORD:-123456}"

SSH_OPTS=(
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
)

NODES=(
  "master0:${MASTER0_IP}:${MASTER0_USER}"
  "master1:${MASTER1_IP}:${MASTER1_USER}"
  "worker0:${WORKER0_IP}:${WORKER0_USER}"
  "worker1:${WORKER1_IP}:${WORKER1_USER}"
)

log() {
  printf '\n[%s] %s\n' "$(date '+%H:%M:%S')" "$*"
}

ssh_node() {
  local user="$1"
  local ip="$2"
  shift 2
  ssh "${SSH_OPTS[@]}" "${user}@${ip}" "$@"
}

sudo_node() {
  local user="$1"
  local ip="$2"
  local command="$3"
  local quoted
  quoted="$(printf "%q" "${command}")"
  ssh_node "${user}" "${ip}" "printf '%s\n' '${SUDO_PASSWORD}' | sudo -S bash -lc ${quoted}"
}

scp_to_node() {
  local src="$1"
  local user="$2"
  local ip="$3"
  local dst="$4"
  scp "${SSH_OPTS[@]}" "$src" "${user}@${ip}:${dst}"
}

build_binaries() {
  log "Building linux/arm64 binaries"
  mkdir -p "${BIN_DIR}"

  (
    cd "${ROOT_DIR}"
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "${BIN_DIR}/astra-scheduler" ./cmd/scheduler
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "${BIN_DIR}/astra-agent" ./cmd/agent
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "${BIN_DIR}/astra-operator" ./cmd/operator
  )
}

write_scheduler_config() {
  mkdir -p "${MANIFEST_DIR}" "${SERVICE_DIR}"

  cat > "${MANIFEST_DIR}/scheduler-config.yaml" <<'YAML'
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: /etc/astra-scheduler/kubeconfig
leaderElection:
  leaderElect: false
profiles:
  - schedulerName: astra-scheduler
    plugins:
      preFilter:
        enabled:
          - name: AstraScheduler
      filter:
        enabled:
          - name: AstraScheduler
      preScore:
        enabled:
          - name: AstraScheduler
      score:
        enabled:
          - name: AstraScheduler
      reserve:
        enabled:
          - name: AstraScheduler
YAML
}

write_demo_manifests() {
  mkdir -p "${MANIFEST_DIR}"

  cat > "${MANIFEST_DIR}/astra-demo.yaml" <<YAML
apiVersion: v1
kind: Namespace
metadata:
  name: ${ASTRA_NAMESPACE}
---
apiVersion: v1
kind: Namespace
metadata:
  name: ${WORKLOAD_NAMESPACE}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: astra-scheduler-fake-node-resources
  namespace: ${ASTRA_NAMESPACE}
data:
  master0.yaml: |
    allocatable:
      gpuCount: 2
      gpuMemoryGiB: 160
      kvCacheGiB: 64
      prefillTokensPerSecond: 4000
      decodeTokensPerSecond: 5000
      totalTokensPerSecond: 9000
    runtime:
      pressure:
        sloRisk: low
        queueDepth: 2
  master1.yaml: |
    allocatable:
      gpuCount: 2
      gpuMemoryGiB: 160
      kvCacheGiB: 48
      prefillTokensPerSecond: 4500
      decodeTokensPerSecond: 3500
      totalTokensPerSecond: 8000
    runtime:
      pressure:
        sloRisk: medium
        queueDepth: 5
  worker0.yaml: |
    allocatable:
      gpuCount: 4
      gpuMemoryGiB: 320
      kvCacheGiB: 128
      prefillTokensPerSecond: 6000
      decodeTokensPerSecond: 9000
      totalTokensPerSecond: 12000
    runtime:
      pressure:
        sloRisk: low
        queueDepth: 1
  worker1.yaml: |
    allocatable:
      gpuCount: 3
      gpuMemoryGiB: 240
      kvCacheGiB: 96
      prefillTokensPerSecond: 9000
      decodeTokensPerSecond: 5000
      totalTokensPerSecond: 14000
    runtime:
      pressure:
        sloRisk: low
        queueDepth: 3
---
apiVersion: astra.aiinfra.io/v1alpha1
kind: AINodeResourceProfile
metadata:
  name: master0
  namespace: ${ASTRA_NAMESPACE}
spec:
  nodeName: master0
  backendType: fake
  gpus:
    - id: gpu-0
      uuid: GPU-fake-master0-0
      product: A100
      memoryGiB: 80
      computeScore: 100
    - id: gpu-1
      uuid: GPU-fake-master0-1
      product: A100
      memoryGiB: 80
      computeScore: 100
  runtime:
    type: fake
    supports:
      kvCache: true
      tokenThroughput: true
      requestThrottling: true
      pauseResume: true
    capacity:
      kvCacheGiB: 64
      tokenThroughput:
        prefillTokensPerSecond: 4000
        decodeTokensPerSecond: 5000
        totalTokensPerSecond: 9000
  capabilities:
    sharing: true
    borrowing: true
    reclaim: true
    throttling: true
    pauseResume: true
    eviction: true
---
apiVersion: astra.aiinfra.io/v1alpha1
kind: AINodeResourceProfile
metadata:
  name: master1
  namespace: ${ASTRA_NAMESPACE}
spec:
  nodeName: master1
  backendType: fake
  gpus:
    - id: gpu-0
      uuid: GPU-fake-master1-0
      product: A100
      memoryGiB: 80
      computeScore: 100
    - id: gpu-1
      uuid: GPU-fake-master1-1
      product: A100
      memoryGiB: 80
      computeScore: 100
  runtime:
    type: fake
    supports:
      kvCache: true
      tokenThroughput: true
      requestThrottling: true
      pauseResume: true
    capacity:
      kvCacheGiB: 48
      tokenThroughput:
        prefillTokensPerSecond: 4500
        decodeTokensPerSecond: 3500
        totalTokensPerSecond: 8000
  capabilities:
    sharing: true
    borrowing: true
    reclaim: true
    throttling: true
    pauseResume: true
    eviction: true
---
apiVersion: astra.aiinfra.io/v1alpha1
kind: AINodeResourceProfile
metadata:
  name: worker0
  namespace: ${ASTRA_NAMESPACE}
spec:
  nodeName: worker0
  backendType: fake
  gpus:
    - id: gpu-0
      uuid: GPU-fake-worker0-0
      product: A100
      memoryGiB: 80
      computeScore: 100
    - id: gpu-1
      uuid: GPU-fake-worker0-1
      product: A100
      memoryGiB: 80
      computeScore: 100
    - id: gpu-2
      uuid: GPU-fake-worker0-2
      product: A100
      memoryGiB: 80
      computeScore: 100
    - id: gpu-3
      uuid: GPU-fake-worker0-3
      product: A100
      memoryGiB: 80
      computeScore: 100
  runtime:
    type: fake
    supports:
      kvCache: true
      tokenThroughput: true
      requestThrottling: true
      pauseResume: true
    capacity:
      kvCacheGiB: 128
      tokenThroughput:
        prefillTokensPerSecond: 6000
        decodeTokensPerSecond: 9000
        totalTokensPerSecond: 12000
  capabilities:
    sharing: true
    borrowing: true
    reclaim: true
    throttling: true
    pauseResume: true
    eviction: true
---
apiVersion: astra.aiinfra.io/v1alpha1
kind: AINodeResourceProfile
metadata:
  name: worker1
  namespace: ${ASTRA_NAMESPACE}
spec:
  nodeName: worker1
  backendType: fake
  gpus:
    - id: gpu-0
      uuid: GPU-fake-worker1-0
      product: A100
      memoryGiB: 80
      computeScore: 100
    - id: gpu-1
      uuid: GPU-fake-worker1-1
      product: A100
      memoryGiB: 80
      computeScore: 100
    - id: gpu-2
      uuid: GPU-fake-worker1-2
      product: A100
      memoryGiB: 80
      computeScore: 100
  runtime:
    type: fake
    supports:
      kvCache: true
      tokenThroughput: true
      requestThrottling: true
      pauseResume: true
    capacity:
      kvCacheGiB: 96
      tokenThroughput:
        prefillTokensPerSecond: 9000
        decodeTokensPerSecond: 5000
        totalTokensPerSecond: 14000
  capabilities:
    sharing: true
    borrowing: true
    reclaim: true
    throttling: true
    pauseResume: true
    eviction: true
---
apiVersion: astra.aiinfra.io/v1alpha1
kind: AIWorkloadProfile
metadata:
  name: live-chat-ai
  namespace: ${WORKLOAD_NAMESPACE}
spec:
  workloadRef:
    apiVersion: apps/v1
    kind: Deployment
    name: live-chat-ai
    namespace: ${WORKLOAD_NAMESPACE}
  workloadType: online
  priority: high
  demandShape: decode_heavy
  timeWindows:
    - name: evening-peak
      start: "20:00"
      end: "02:00"
      timezone: America/New_York
  slo:
    ttftP95Ms: 800
    latencyP99Ms: 3000
  resources:
    required:
      gpuCount: 1
      gpuMemoryGiB: 10
      kvCacheGiB: 16
      decodeTokensPerSecond: 1000
    preferred:
      gpuCount: 1
      gpuMemoryGiB: 20
      kvCacheGiB: 32
      decodeTokensPerSecond: 2500
    max:
      gpuCount: 1
      gpuMemoryGiB: 30
      kvCacheGiB: 48
      decodeTokensPerSecond: 4000
  policy:
    allowBorrowing: false
    allowReclaimFromLowerPriority: true
    throttleable: false
    pauseable: false
    evictable: false
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: live-chat-ai
  namespace: ${WORKLOAD_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: live-chat-ai
  template:
    metadata:
      labels:
        app: live-chat-ai
      annotations:
        astra.aiinfra.io/workload-profile: live-chat-ai
    spec:
      schedulerName: astra-scheduler
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10.1
          imagePullPolicy: IfNotPresent
YAML
}

write_systemd_units() {
  mkdir -p "${SERVICE_DIR}"

  cat > "${SERVICE_DIR}/astra-scheduler.service" <<EOF
[Unit]
Description=Astra Scheduler
After=network-online.target
Wants=network-online.target

[Service]
Environment=ASTRA_NODE_RESOURCE_NAMESPACE=${ASTRA_NAMESPACE}
ExecStart=${REMOTE_ROOT}/bin/astra-scheduler --config=${REMOTE_ETC}/scheduler-config.yaml --kubeconfig=${REMOTE_ETC}/kubeconfig --secure-port=10260 --v=4
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

  cat > "${SERVICE_DIR}/astra-operator.service" <<EOF
[Unit]
Description=Astra Operator
After=network-online.target
Wants=network-online.target

[Service]
Environment=KUBECONFIG=${REMOTE_ETC}/kubeconfig
ExecStart=${REMOTE_ROOT}/bin/astra-operator --metrics-bind-address=0 --health-probe-bind-address=:8082
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

  cat > "${SERVICE_DIR}/astra-agent@.service" <<EOF
[Unit]
Description=Astra Agent for %i
After=network-online.target
Wants=network-online.target

[Service]
Environment=KUBECONFIG=${REMOTE_ETC}/kubeconfig
Environment=NODE_NAME=%i
ExecStart=${REMOTE_ROOT}/bin/astra-agent --backend=fake --node-name=%i --node-profile-namespace=${ASTRA_NAMESPACE} --fake-resource-configmap=astra-scheduler-fake-node-resources --fake-resource-configmap-namespace=${ASTRA_NAMESPACE} --metrics-bind-address=0 --health-probe-bind-address=:8081
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
}

install_binaries_and_services() {
  log "Installing binaries and services on all nodes"

  for entry in "${NODES[@]}"; do
    IFS=":" read -r node ip user <<< "${entry}"
    log "Preparing ${node} (${user}@${ip})"
    sudo_node "${user}" "${ip}" "mkdir -p '${REMOTE_ROOT}/bin' '${REMOTE_ETC}' /tmp/astra-scheduler-deploy"
    scp_to_node "${BIN_DIR}/astra-agent" "${user}" "${ip}" "/tmp/astra-agent"
    scp_to_node "${BIN_DIR}/astra-scheduler" "${user}" "${ip}" "/tmp/astra-scheduler"
    scp_to_node "${BIN_DIR}/astra-operator" "${user}" "${ip}" "/tmp/astra-operator"
    scp_to_node "${SERVICE_DIR}/astra-agent@.service" "${user}" "${ip}" "/tmp/astra-agent@.service"
    sudo_node "${user}" "${ip}" "install -m 0755 /tmp/astra-agent '${REMOTE_ROOT}/bin/astra-agent' && install -m 0755 /tmp/astra-scheduler '${REMOTE_ROOT}/bin/astra-scheduler' && install -m 0755 /tmp/astra-operator '${REMOTE_ROOT}/bin/astra-operator' && install -m 0644 /tmp/astra-agent@.service /etc/systemd/system/astra-agent@.service"
  done

  scp_to_node "${SERVICE_DIR}/astra-scheduler.service" "${MASTER0_USER}" "${MASTER0_IP}" "/tmp/astra-scheduler.service"
  scp_to_node "${SERVICE_DIR}/astra-operator.service" "${MASTER0_USER}" "${MASTER0_IP}" "/tmp/astra-operator.service"
  scp_to_node "${MANIFEST_DIR}/scheduler-config.yaml" "${MASTER0_USER}" "${MASTER0_IP}" "/tmp/scheduler-config.yaml"
  sudo_node "${MASTER0_USER}" "${MASTER0_IP}" "install -m 0644 /tmp/astra-scheduler.service /etc/systemd/system/astra-scheduler.service && install -m 0644 /tmp/astra-operator.service /etc/systemd/system/astra-operator.service && install -m 0644 /tmp/scheduler-config.yaml '${REMOTE_ETC}/scheduler-config.yaml'"
}

install_kubeconfig() {
  log "Copying kubeconfig from master0 to all nodes"
  sudo_node "${MASTER0_USER}" "${MASTER0_IP}" "cp '${KUBECONFIG_PATH}' /tmp/astra-kubeconfig && chown '${MASTER0_USER}:${MASTER0_USER}' /tmp/astra-kubeconfig"

  local local_kubeconfig="${DIST_DIR}/kubeconfig"
  scp "${SSH_OPTS[@]}" "${MASTER0_USER}@${MASTER0_IP}:/tmp/astra-kubeconfig" "${local_kubeconfig}"

  for entry in "${NODES[@]}"; do
    IFS=":" read -r _node ip user <<< "${entry}"
    scp_to_node "${local_kubeconfig}" "${user}" "${ip}" "/tmp/astra-kubeconfig"
    sudo_node "${user}" "${ip}" "install -m 0600 /tmp/astra-kubeconfig '${REMOTE_ETC}/kubeconfig'"
  done
}

apply_kubernetes_resources() {
  log "Applying CRDs and demo resources"
  scp_to_node "${MANIFEST_DIR}/astra-demo.yaml" "${MASTER0_USER}" "${MASTER0_IP}" "/tmp/astra-demo.yaml"

  tar -C "${ROOT_DIR}" -czf "${DIST_DIR}/config-crd.tgz" config/crd
  scp_to_node "${DIST_DIR}/config-crd.tgz" "${MASTER0_USER}" "${MASTER0_IP}" "/tmp/astra-config-crd.tgz"
  ssh_node "${MASTER0_USER}" "${MASTER0_IP}" "rm -rf /tmp/astra-config-crd && mkdir -p /tmp/astra-config-crd && tar -C /tmp/astra-config-crd -xzf /tmp/astra-config-crd.tgz && kubectl apply -k /tmp/astra-config-crd/config/crd && kubectl apply -f /tmp/astra-demo.yaml"
}

start_services() {
  log "Starting systemd services"

  sudo_node "${MASTER0_USER}" "${MASTER0_IP}" "systemctl daemon-reload && systemctl enable --now astra-scheduler.service astra-operator.service"

  for entry in "${NODES[@]}"; do
    IFS=":" read -r node ip user <<< "${entry}"
    sudo_node "${user}" "${ip}" "systemctl daemon-reload && systemctl enable --now 'astra-agent@${node}.service'"
  done
}

show_status() {
  log "Cluster status"
  ssh_node "${MASTER0_USER}" "${MASTER0_IP}" "kubectl get nodes -o wide && kubectl get ainoderesourceprofiles -A && kubectl get aiworkloadprofiles -A && sleep 3 && kubectl get pods -n '${WORKLOAD_NAMESPACE}' -o wide && kubectl get airesourceallocations -A -o wide"

  log "Service status on master0"
  ssh_node "${MASTER0_USER}" "${MASTER0_IP}" "systemctl --no-pager --full status astra-scheduler.service astra-operator.service | sed -n '1,120p'"
}

main() {
  build_binaries
  write_scheduler_config
  write_demo_manifests
  write_systemd_units
  install_binaries_and_services
  install_kubeconfig
  apply_kubernetes_resources
  start_services
  show_status

  log "Done"
  cat <<EOF

Useful commands:
  ssh ${MASTER0_USER}@${MASTER0_IP} 'kubectl get pods -n ${WORKLOAD_NAMESPACE} -o wide'
  ssh ${MASTER0_USER}@${MASTER0_IP} 'kubectl get airesourceallocations -A -o yaml'
  ssh ${MASTER0_USER}@${MASTER0_IP} 'sudo journalctl -u astra-scheduler -f'
  ssh ${WORKER0_USER}@${WORKER0_IP} 'sudo journalctl -u astra-agent@worker0 -f'

EOF
}

main "$@"

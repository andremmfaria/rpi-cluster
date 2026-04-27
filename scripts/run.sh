#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SETUP_DIR="$REPO_ROOT/cluster-setup"
PLATFORM_DIR="$REPO_ROOT/cluster-platform"

KUBECONFIG="${KUBECONFIG:-$HOME/.kube/rpi-cluster.yaml}"
export KUBECONFIG

RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()    { echo -e "${CYAN}==>${RESET} $*"; }
success() { echo -e "${GREEN}✔${RESET}  $*"; }
warn()    { echo -e "${YELLOW}⚠${RESET}  $*"; }
error()   { echo -e "${RED}✖${RESET}  $*" >&2; }
header()  { echo -e "\n${BOLD}$*${RESET}"; }

usage() {
  cat <<EOF
${BOLD}rpi-cluster management wrapper${RESET}

Usage: $(basename "$0") <module> <command> [options]

${BOLD}Modules:${RESET}
  setup      Ansible — provision and bootstrap nodes
  platform   kubectl — manage cluster infrastructure
  cluster    cross-cutting cluster operations

${BOLD}setup commands:${RESET}
  deps          Install required Ansible Galaxy collections
  ping          Pre-flight connectivity and hardware check
  deploy        Bootstrap the full cluster (site.yml)
  reset         Tear down k3s — DESTRUCTIVE, requires confirmation
  check         Dry-run deploy (--check --diff)
  lint          Run yamllint + ansible-lint

  Options: -i/--inventory  -l/--limit  -t/--tags  --skip-tags
           -e/--extra-vars  --vault-pass-file  --ask-vault-pass  -v/-vv/-vvv

${BOLD}platform commands:${RESET}
  kubeconfig    Fetch kubeconfig from a control-plane node
                Required: --server <ip> --user <user> --key <ssh-key>
                Optional: --out <path>  (default: ~/.kube/rpi-cluster.yaml)
  apply <comp>  Apply component (kube-vip | metallb | ingress-nginx | longhorn | all)
  diff  <comp>  Dry-run diff against live cluster
  delete <comp> Remove a component — requires confirmation
  status        Show pod/service status across all platform namespaces
  lint          Run yamllint on cluster-platform manifests

${BOLD}cluster commands:${RESET}
  shutdown      Gracefully drain and power off all nodes

${BOLD}Examples:${RESET}
  $(basename "$0") setup deps
  $(basename "$0") setup ping
  $(basename "$0") setup deploy
  $(basename "$0") setup deploy -l rpi-0 -t common -vv
  $(basename "$0") setup check
  $(basename "$0") setup reset
  $(basename "$0") platform kubeconfig --server 192.168.50.20 --user rpi --key ~/.ssh/id_rpi
  $(basename "$0") platform apply all
  $(basename "$0") platform apply longhorn
  $(basename "$0") platform diff kube-vip
  $(basename "$0") platform status
  $(basename "$0") cluster shutdown

EOF
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

require_value() {
  local opt="$1" val="${2:-}"
  [[ -z "$val" ]] && { error "Option '$opt' requires a value."; exit 1; }
}

check_kubectl() {
  command -v kubectl &>/dev/null || { error "kubectl not found."; exit 1; }
  kubectl cluster-info &>/dev/null 2>&1 || {
    error "Cannot reach cluster. Run: $(basename "$0") platform kubeconfig ..."
    exit 1
  }
}

# ---------------------------------------------------------------------------
# setup — Ansible
# ---------------------------------------------------------------------------

INVENTORY="homelab"
LIMIT=""
TAGS=""
SKIP_TAGS=""
VERBOSITY=""
CHECK_MODE="false"
VAULT_ARGS=()
EXTRA_VARS=()

parse_setup_opts() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -i|--inventory)    require_value "$1" "${2:-}"; INVENTORY="$2"; shift 2 ;;
      -l|--limit)        require_value "$1" "${2:-}"; LIMIT="$2";     shift 2 ;;
      -t|--tags)         require_value "$1" "${2:-}"; TAGS="$2";      shift 2 ;;
      --skip-tags)       require_value "$1" "${2:-}"; SKIP_TAGS="$2"; shift 2 ;;
      -e|--extra-vars)   require_value "$1" "${2:-}"; EXTRA_VARS+=("$2"); shift 2 ;;
      --vault-pass-file) require_value "$1" "${2:-}"; VAULT_ARGS+=(--vault-password-file "$2"); shift 2 ;;
      --ask-vault-pass)  VAULT_ARGS+=(--ask-vault-pass); shift ;;
      -v)   VERBOSITY="-v";   shift ;;
      -vv)  VERBOSITY="-vv";  shift ;;
      -vvv) VERBOSITY="-vvv"; shift ;;
      -h|--help) usage; exit 0 ;;
      *) error "Unknown option: '$1'"; exit 1 ;;
    esac
  done
}

run_playbook() {
  local playbook="$1"; shift
  local inventory_path="inventories/${INVENTORY}/hosts.yml"

  [[ -f "$inventory_path" ]] || { error "Inventory not found: $inventory_path"; exit 1; }

  local cmd=(ansible-playbook -i "$inventory_path" "$playbook")
  [[ "$CHECK_MODE" == "true" ]] && cmd+=(--check --diff)
  [[ -n "$LIMIT" ]]             && cmd+=(-l "$LIMIT")
  [[ -n "$TAGS" ]]              && cmd+=(-t "$TAGS")
  [[ -n "$SKIP_TAGS" ]]         && cmd+=(--skip-tags "$SKIP_TAGS")
  [[ -n "$VERBOSITY" ]]         && cmd+=("$VERBOSITY")
  [[ ${#VAULT_ARGS[@]} -gt 0 ]] && cmd+=("${VAULT_ARGS[@]}")
  for var in "${EXTRA_VARS[@]+"${EXTRA_VARS[@]}"}"; do cmd+=(-e "$var"); done
  cmd+=("$@")

  info "Running: ${cmd[*]}"
  echo
  "${cmd[@]}"
}

setup_deps() {
  header "Installing Ansible Galaxy collections"
  ansible-galaxy collection install community.general ansible.posix
  success "Dependencies installed."
}

setup_ping() {
  header "Pre-flight validation — inventory: ${INVENTORY}"
  run_playbook playbooks/00-ping.yml
}

setup_deploy() {
  header "Deploying cluster — inventory: ${INVENTORY}"
  run_playbook site.yml
}

setup_reset() {
  header "Cluster reset — inventory: ${INVENTORY}"
  warn "This will DESTROY the k3s cluster and wipe all node data."
  echo
  read -r -p "  Type 'yes' to confirm: " confirm; echo
  [[ "$confirm" == "yes" ]] || { info "Aborted."; exit 0; }
  run_playbook playbooks/99-reset.yml
}

setup_check() {
  header "Dry-run (check + diff) — inventory: ${INVENTORY}"
  CHECK_MODE="true"
  run_playbook site.yml
}

setup_lint() {
  header "Linting cluster-setup"
  local errors=0
  info "Running yamllint..."
  yamllint . && success "yamllint passed." || { error "yamllint failed."; errors=$((errors+1)); }
  echo
  info "Running ansible-lint..."
  ansible-lint && success "ansible-lint passed." || { error "ansible-lint failed."; errors=$((errors+1)); }
  echo
  [[ $errors -gt 0 ]] && { error "$errors linter(s) failed."; exit 1; }
  success "All linters passed."
}

cmd_setup() {
  local command="${1:-}"; shift || true
  parse_setup_opts "$@"

  cd "$SETUP_DIR"
  case "$command" in
    deps)   setup_deps   ;;
    ping)   setup_ping   ;;
    deploy) setup_deploy ;;
    reset)  setup_reset  ;;
    check)  setup_check  ;;
    lint)   setup_lint   ;;
    "") usage; exit 1 ;;
    *) error "Unknown setup command: '$command'"; exit 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# platform — kubectl
# ---------------------------------------------------------------------------

platform_kubeconfig() {
  local server="" ssh_user="" ssh_key="" out="$HOME/.kube/rpi-cluster.yaml"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --server) server="$2";   shift 2 ;;
      --user)   ssh_user="$2"; shift 2 ;;
      --key)    ssh_key="$2";  shift 2 ;;
      --out)    out="$2";      shift 2 ;;
      *) error "Unknown option: '$1'"; exit 1 ;;
    esac
  done

  [[ -z "$server"   ]] && { error "--server <ip> is required";            exit 1; }
  [[ -z "$ssh_user" ]] && { error "--user <ssh-user> is required";        exit 1; }
  [[ -z "$ssh_key"  ]] && { error "--key <path-to-private-key> required"; exit 1; }
  [[ -f "$ssh_key"  ]] || { error "SSH key not found: $ssh_key";          exit 1; }

  header "Fetching kubeconfig from ${ssh_user}@${server}"
  mkdir -p "$(dirname "$out")"
  ssh -i "$ssh_key" -o StrictHostKeyChecking=no "${ssh_user}@${server}" \
    "sudo cat /etc/rancher/k3s/k3s.yaml" \
    | sed "s|https://127.0.0.1:6443|https://${server}:6443|g" > "$out"
  chmod 600 "$out"
  success "Saved to $out"
  info "Export: export KUBECONFIG=$out"
}

apply_component() {
  local component="$1"
  case "$component" in
    kube-vip)
      info "Applying kube-vip..."
      kubectl apply -f infrastructure/kube-vip/
      success "kube-vip applied."
      ;;
    metallb)
      info "Applying MetalLB controllers..."
      kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.15.3/config/manifests/metallb-native.yaml
      info "Waiting for MetalLB CRDs..."
      kubectl wait --for=condition=Established \
        crd/ipaddresspools.metallb.io crd/l2advertisements.metallb.io --timeout=60s
      info "Applying MetalLB pool..."
      kubectl apply -f infrastructure/metallb/pool.yaml
      success "MetalLB applied."
      ;;
    ingress-nginx)
      info "Applying ingress-nginx..."
      kubectl apply -f infrastructure/ingress-nginx/
      success "ingress-nginx applied."
      ;;
    longhorn)
      info "Applying Longhorn namespace..."
      kubectl apply -f infrastructure/longhorn/namespace.yaml
      info "Applying Longhorn Helm release..."
      kubectl apply -f infrastructure/longhorn/helm-release.yaml
      info "Waiting for Longhorn CRDs (~2 min)..."
      local deadline=$(( $(date +%s) + 300 ))
      until kubectl get crd volumes.longhorn.io nodes.longhorn.io settings.longhorn.io \
              &>/dev/null 2>&1; do
        [[ $(date +%s) -ge $deadline ]] && { error "Timed out waiting for Longhorn CRDs."; exit 1; }
        sleep 10; info "  Still waiting..."
      done
      kubectl wait --for=condition=Established \
        crd/volumes.longhorn.io crd/nodes.longhorn.io crd/settings.longhorn.io --timeout=60s
      info "Applying Longhorn UI ingress..."
      kubectl apply -f infrastructure/longhorn/ingress.yaml
      success "Longhorn applied."
      ;;
    all)
      warn "Applying all: kube-vip → metallb → ingress-nginx → longhorn"
      echo; apply_component kube-vip
      echo; apply_component metallb
      echo; apply_component ingress-nginx
      echo; apply_component longhorn
      ;;
    *) error "Unknown component: '$component'"; error "Valid: kube-vip | metallb | ingress-nginx | longhorn | all"; exit 1 ;;
  esac
}

diff_component() {
  local component="$1"
  case "$component" in
    kube-vip)     kubectl diff -f infrastructure/kube-vip/ || true ;;
    metallb)
      kubectl diff -f https://raw.githubusercontent.com/metallb/metallb/v0.15.3/config/manifests/metallb-native.yaml || true
      kubectl diff -f infrastructure/metallb/pool.yaml || true
      ;;
    ingress-nginx) kubectl diff -f infrastructure/ingress-nginx/ || true ;;
    longhorn)
      kubectl diff -f infrastructure/longhorn/namespace.yaml || true
      kubectl diff -f infrastructure/longhorn/helm-release.yaml || true
      kubectl diff -f infrastructure/longhorn/ingress.yaml 2>/dev/null || true
      ;;
    all)
      diff_component kube-vip; echo
      diff_component metallb; echo
      diff_component ingress-nginx; echo
      diff_component longhorn
      ;;
    *) error "Unknown component: '$component'"; exit 1 ;;
  esac
}

delete_component() {
  local component="$1"
  warn "This will DELETE $component from the cluster."
  echo
  read -r -p "  Type 'yes' to confirm: " confirm; echo
  [[ "$confirm" == "yes" ]] || { info "Aborted."; exit 0; }
  case "$component" in
    kube-vip)     kubectl delete -f infrastructure/kube-vip/ --ignore-not-found; success "kube-vip deleted." ;;
    metallb)
      kubectl delete -f infrastructure/metallb/pool.yaml --ignore-not-found
      kubectl delete -f https://raw.githubusercontent.com/metallb/metallb/v0.15.3/config/manifests/metallb-native.yaml --ignore-not-found
      success "MetalLB deleted."
      ;;
    ingress-nginx) kubectl delete -f infrastructure/ingress-nginx/ --ignore-not-found; success "ingress-nginx deleted." ;;
    longhorn)
      kubectl delete -f infrastructure/longhorn/ingress.yaml --ignore-not-found
      kubectl delete -f infrastructure/longhorn/helm-release.yaml --ignore-not-found
      kubectl delete -f infrastructure/longhorn/namespace.yaml --ignore-not-found
      success "Longhorn deleted."
      ;;
    *) error "Unknown component: '$component'"; exit 1 ;;
  esac
}

platform_status() {
  check_kubectl
  header "Platform status"
  echo; info "kube-vip (kube-system)"
  kubectl get pods -n kube-system -l app=kube-vip 2>/dev/null || echo "  not found"
  echo; info "MetalLB (metallb-system)"
  kubectl get pods -n metallb-system 2>/dev/null || echo "  not found"
  echo; info "ingress-nginx"
  kubectl get pods,svc -n ingress-nginx 2>/dev/null || echo "  not found"
  echo; info "Longhorn (longhorn-system)"
  kubectl get pods -n longhorn-system 2>/dev/null || echo "  not found"
  echo; info "Longhorn StorageClass"
  kubectl get storageclass longhorn 2>/dev/null || echo "  not found"
}

platform_lint() {
  header "Linting cluster-platform manifests"
  command -v yamllint &>/dev/null || { error "yamllint not found. Run: pip install yamllint"; exit 1; }
  yamllint . && success "yamllint passed." || { error "yamllint failed."; exit 1; }
}

cmd_platform() {
  local command="${1:-}"; shift || true

  cd "$PLATFORM_DIR"
  case "$command" in
    kubeconfig) platform_kubeconfig "$@" ;;
    apply)
      check_kubectl
      [[ $# -eq 0 ]] && { error "apply requires a component."; exit 1; }
      header "Apply — ${1}"; apply_component "$1"
      ;;
    diff)
      check_kubectl
      [[ $# -eq 0 ]] && { error "diff requires a component."; exit 1; }
      header "Diff — ${1}"; diff_component "$1"
      ;;
    delete)
      check_kubectl
      [[ $# -eq 0 ]] && { error "delete requires a component."; exit 1; }
      header "Delete — ${1}"; delete_component "$1"
      ;;
    status) platform_status ;;
    lint)   platform_lint   ;;
    "") usage; exit 1 ;;
    *) error "Unknown platform command: '$command'"; exit 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# cluster — cross-cutting
# ---------------------------------------------------------------------------

SSH_USER="rpi"
SSH_KEY="$HOME/.ssh/id_rpi"
SSH_OPTS="-i $SSH_KEY -o StrictHostKeyChecking=no -o ConnectTimeout=10"

AGENTS=(rpi-3 rpi-4 rpi-5)
AGENT_IPS=(192.168.50.23 192.168.50.24 192.168.50.25)
JOINING_SERVERS=(rpi-2 rpi-1)
JOINING_SERVER_IPS=(192.168.50.22 192.168.50.21)
INIT_SERVER="rpi-0"
INIT_SERVER_IP="192.168.50.20"

drain_node() {
  local node="$1"
  info "Draining $node..."
  kubectl drain "$node" \
    --ignore-daemonsets \
    --delete-emptydir-data \
    --force \
    --timeout=120s \
    --grace-period=30 2>&1 | grep -v "^Warning"
  success "$node drained."
}

ssh_shutdown() {
  local label="$1" ip="$2"
  ssh $SSH_OPTS "${SSH_USER}@${ip}" "sudo shutdown -h now" 2>/dev/null && \
    info "  Shutdown sent to $label ($ip)" || \
    warn "  Could not reach $label ($ip) — may already be offline"
}

cluster_shutdown() {
  check_kubectl

  header "Current cluster state"
  kubectl get nodes -o wide
  echo

  warn "This will GRACEFULLY SHUT DOWN all 6 Raspberry Pi nodes."
  warn "All workloads will be evicted. The cluster will be offline."
  echo
  read -r -p "  Type 'shutdown' to confirm: " answer; echo
  [[ "$answer" == "shutdown" ]] || { info "Aborted."; exit 0; }

  header "Cordoning all nodes"
  for node in "${AGENTS[@]}" "${JOINING_SERVERS[@]}" "$INIT_SERVER"; do
    kubectl cordon "$node" && info "  Cordoned $node"
  done

  header "Draining agents (parallel)"
  local pids=()
  for node in "${AGENTS[@]}"; do drain_node "$node" & pids+=($!); done
  for pid in "${pids[@]}"; do wait "$pid"; done
  success "All agents drained."

  header "Draining joining servers (serial — etcd quorum)"
  for node in "${JOINING_SERVERS[@]}"; do drain_node "$node"; done

  header "Draining init server (rpi-0)"
  drain_node "$INIT_SERVER"

  header "Shutting down nodes"
  info "Agents (simultaneous)..."
  for i in "${!AGENTS[@]}"; do ssh_shutdown "${AGENTS[$i]}" "${AGENT_IPS[$i]}" & done
  wait; sleep 5

  info "Joining servers (simultaneous)..."
  for i in "${!JOINING_SERVERS[@]}"; do ssh_shutdown "${JOINING_SERVERS[$i]}" "${JOINING_SERVER_IPS[$i]}" & done
  wait; sleep 5

  info "Init server (rpi-0)..."
  ssh_shutdown "$INIT_SERVER" "$INIT_SERVER_IP"

  echo
  success "All nodes shutting down. Cluster is offline."
}

cmd_cluster() {
  local command="${1:-}"; shift || true
  case "$command" in
    shutdown) cluster_shutdown ;;
    "") usage; exit 1 ;;
    *) error "Unknown cluster command: '$command'"; exit 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

main() {
  if [[ $# -eq 0 ]]; then usage; exit 1; fi

  local module="$1"; shift
  case "$module" in
    setup)   cmd_setup   "$@" ;;
    platform) cmd_platform "$@" ;;
    cluster) cmd_cluster "$@" ;;
    -h|--help) usage; exit 0 ;;
    *) error "Unknown module: '$module'"; usage; exit 1 ;;
  esac
}

main "$@"

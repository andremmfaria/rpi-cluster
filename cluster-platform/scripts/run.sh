#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

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
${BOLD}cluster-platform kubectl wrapper${RESET}

Usage: $(basename "$0") <command> [options]

${BOLD}Commands:${RESET}
  kubeconfig          Fetch kubeconfig from a control-plane node
                      Required: --server <ip> --user <ssh-user> --key <ssh-key-path>
                      Optional: --out <path>  (default: ~/.kube/rpi-cluster.yaml)
  apply <component>   Apply a specific component (kube-vip | metallb | ingress-nginx | all)
  diff  <component>   Dry-run diff for a component (kube-vip | metallb | ingress-nginx | all)
  delete <component>  Remove a component — DESTRUCTIVE, requires confirmation
  lint                Run yamllint on all manifests
  status              Show pod/service status for all platform namespaces

${BOLD}Examples:${RESET}
  $(basename "$0") kubeconfig --server 192.168.50.20 --user rpi --key ~/.ssh/id_rpi
  $(basename "$0") kubeconfig --server 192.168.50.30 --user rpi --key ~/.ssh/id_rpi --out ~/.kube/rpi-vip.yaml
  $(basename "$0") apply kube-vip
  $(basename "$0") apply all
  $(basename "$0") diff metallb
  $(basename "$0") delete ingress-nginx
  $(basename "$0") status
  $(basename "$0") lint

EOF
}

check_kubectl() {
  if ! command -v kubectl &>/dev/null; then
    error "kubectl not found. Install it first."
    exit 1
  fi
  if ! kubectl cluster-info &>/dev/null 2>&1; then
    error "Cannot reach cluster. Run: $(basename "$0") kubeconfig"
    exit 1
  fi
}

cmd_kubeconfig() {
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

  [[ -z "$server"   ]] && { error "--server <ip> is required";       exit 1; }
  [[ -z "$ssh_user" ]] && { error "--user <ssh-user> is required";    exit 1; }
  [[ -z "$ssh_key"  ]] && { error "--key <path-to-private-key> is required"; exit 1; }

  header "Fetching kubeconfig from ${ssh_user}@${server}"

  if [[ ! -f "$ssh_key" ]]; then
    error "SSH key not found: $ssh_key"
    exit 1
  fi

  mkdir -p "$(dirname "$out")"

  ssh -i "$ssh_key" -o StrictHostKeyChecking=no "${ssh_user}@${server}" \
    "sudo cat /etc/rancher/k3s/k3s.yaml" \
    | sed "s|https://127.0.0.1:6443|https://${server}:6443|g" \
    > "$out"

  chmod 600 "$out"
  success "Saved to $out"
  info "Export: export KUBECONFIG=$out"
  info "Or merge into ~/.kube/config with: KUBECONFIG=~/.kube/config:$out kubectl config view --flatten > /tmp/merged && mv /tmp/merged ~/.kube/config"
}

cmd_lint() {
  header "Linting manifests"
  if ! command -v yamllint &>/dev/null; then
    error "yamllint not found. Run: pip install yamllint"
    exit 1
  fi
  if yamllint .; then
    success "yamllint passed."
  else
    error "yamllint failed."
    exit 1
  fi
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
      info "Waiting for MetalLB CRDs to be established..."
      kubectl wait --for=condition=Established \
        crd/ipaddresspools.metallb.io \
        crd/l2advertisements.metallb.io \
        --timeout=60s
      info "Applying MetalLB pool configuration..."
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
      info "Waiting for Longhorn CRDs to appear (this takes ~2 min)..."
      local deadline=$(( $(date +%s) + 300 ))
      until kubectl get crd volumes.longhorn.io nodes.longhorn.io settings.longhorn.io \
              &>/dev/null 2>&1; do
        if [[ $(date +%s) -ge $deadline ]]; then
          error "Timed out waiting for Longhorn CRDs after 5 minutes"
          exit 1
        fi
        sleep 10
        info "  Still waiting..."
      done
      kubectl wait --for=condition=Established \
        crd/volumes.longhorn.io \
        crd/nodes.longhorn.io \
        crd/settings.longhorn.io \
        --timeout=60s
      info "Applying Longhorn UI ingress..."
      kubectl apply -f infrastructure/longhorn/ingress.yaml
      success "Longhorn applied."
      ;;
    all)
      warn "Applying all components in order: kube-vip → metallb → ingress-nginx → longhorn"
      echo
      apply_component kube-vip
      echo
      apply_component metallb
      echo
      apply_component ingress-nginx
      echo
      apply_component longhorn
      ;;
    *)
      error "Unknown component: '$component'"
      error "Valid: kube-vip | metallb | ingress-nginx | longhorn | all"
      exit 1
      ;;
  esac
}

diff_component() {
  local component="$1"
  case "$component" in
    kube-vip)
      info "Diffing kube-vip..."
      kubectl diff -f infrastructure/kube-vip/ || true
      ;;
    metallb)
      info "Diffing MetalLB controllers..."
      kubectl diff -f https://raw.githubusercontent.com/metallb/metallb/v0.15.3/config/manifests/metallb-native.yaml || true
      info "Diffing MetalLB pool configuration..."
      kubectl diff -f infrastructure/metallb/pool.yaml || true
      ;;
    ingress-nginx)
      info "Diffing ingress-nginx..."
      kubectl diff -f infrastructure/ingress-nginx/ || true
      ;;
    longhorn)
      info "Diffing Longhorn..."
      kubectl diff -f infrastructure/longhorn/namespace.yaml || true
      kubectl diff -f infrastructure/longhorn/helm-release.yaml || true
      kubectl diff -f infrastructure/longhorn/ingress.yaml 2>/dev/null || true
      ;;
    all)
      diff_component kube-vip
      echo
      diff_component metallb
      echo
      diff_component ingress-nginx
      echo
      diff_component longhorn
      ;;
    *)
      error "Unknown component: '$component'"
      error "Valid: kube-vip | metallb | ingress-nginx | longhorn | all"
      exit 1
      ;;
  esac
}

delete_component() {
  local component="$1"
  warn "This will DELETE $component from the cluster."
  echo
  read -r -p "  Type 'yes' to confirm: " confirm
  echo
  if [[ "$confirm" != "yes" ]]; then
    info "Aborted."
    exit 0
  fi
  case "$component" in
    kube-vip)
      kubectl delete -f infrastructure/kube-vip/ --ignore-not-found
      success "kube-vip deleted."
      ;;
    metallb)
      kubectl delete -f infrastructure/metallb/pool.yaml --ignore-not-found
      kubectl delete -f https://raw.githubusercontent.com/metallb/metallb/v0.15.3/config/manifests/metallb-native.yaml --ignore-not-found
      success "MetalLB deleted."
      ;;
    ingress-nginx)
      kubectl delete -f infrastructure/ingress-nginx/ --ignore-not-found
      success "ingress-nginx deleted."
      ;;
    longhorn)
      kubectl delete -f infrastructure/longhorn/ingress.yaml --ignore-not-found
      kubectl delete -f infrastructure/longhorn/helm-release.yaml --ignore-not-found
      kubectl delete -f infrastructure/longhorn/namespace.yaml --ignore-not-found
      success "Longhorn deleted."
      ;;
    *)
      error "Unknown component: '$component'"
      error "Valid: kube-vip | metallb | ingress-nginx | longhorn"
      exit 1
      ;;
  esac
}

cmd_status() {
  check_kubectl
  header "Platform status"
  echo
  info "kube-vip (kube-system)"
  kubectl get pods -n kube-system -l app=kube-vip 2>/dev/null || echo "  not found"
  echo
  info "MetalLB (metallb-system)"
  kubectl get pods -n metallb-system 2>/dev/null || echo "  not found"
  echo
  info "ingress-nginx"
  kubectl get pods,svc -n ingress-nginx 2>/dev/null || echo "  not found"
  echo
  info "Longhorn (longhorn-system)"
  kubectl get pods -n longhorn-system 2>/dev/null || echo "  not found"
  echo
  info "Longhorn StorageClass"
  kubectl get storageclass longhorn 2>/dev/null || echo "  not found"
}

main() {
  if [[ $# -eq 0 ]]; then
    usage
    exit 1
  fi

  local command="$1"
  shift

  case "$command" in
    apply)
      check_kubectl
      [[ $# -eq 0 ]] && { error "apply requires a component argument."; usage; exit 1; }
      header "Apply — ${1}"
      apply_component "$1"
      ;;
    diff)
      check_kubectl
      [[ $# -eq 0 ]] && { error "diff requires a component argument."; usage; exit 1; }
      header "Diff — ${1}"
      diff_component "$1"
      ;;
    delete)
      check_kubectl
      [[ $# -eq 0 ]] && { error "delete requires a component argument."; usage; exit 1; }
      header "Delete — ${1}"
      delete_component "$1"
      ;;
    lint)       cmd_lint   ;;
    status)     cmd_status ;;
    kubeconfig) cmd_kubeconfig "$@" ;;
    -h|--help) usage; exit 0 ;;
    *)
      error "Unknown command: '$command'"
      usage
      exit 1
      ;;
  esac
}

main "$@"

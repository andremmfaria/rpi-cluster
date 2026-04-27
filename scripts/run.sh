#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SETUP_DIR="$REPO_ROOT/cluster-setup"
CLI_DIR="$REPO_ROOT/cluster-cli"

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
  cli        cluster-cli Go binary — build, test, install

  Platform and cluster operations → use ./bin/rpicli

${BOLD}setup commands:${RESET}
  deps          Install required Ansible Galaxy collections
  ping          Pre-flight connectivity and hardware check
  deploy        Bootstrap the full cluster (site.yml)
  reset         Tear down k3s — DESTRUCTIVE, requires confirmation
  check         Dry-run deploy (--check --diff)
  lint          Run yamllint + ansible-lint

  Options: -i/--inventory  -l/--limit  -t/--tags  --skip-tags
           -e/--extra-vars  --vault-pass-file  --ask-vault-pass  -v/-vv/-vvv

${BOLD}cli commands:${RESET}
  build [--all]    Build rpicli binary to ./bin/rpicli
  test             Run unit tests with coverage
  lint             go vet
  install [path]   Build and install to /usr/local/bin/rpicli

${BOLD}Examples:${RESET}
  $(basename "$0") setup deps
  $(basename "$0") setup ping
  $(basename "$0") setup deploy
  $(basename "$0") setup deploy -l rpi-0 -t common -vv
  $(basename "$0") setup check
  $(basename "$0") setup reset
  $(basename "$0") cli build
  $(basename "$0") cli build --all
  $(basename "$0") cli test

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
# cli — Go binary build/test
# ---------------------------------------------------------------------------

cmd_cli() {
  local command="${1:-}"; shift || true
  case "$command" in
    build)   cli_build   "$@" ;;
    test)    cli_test       ;;
    lint)    cli_lint       ;;
    install) cli_install "$@" ;;
    "") usage; exit 1 ;;
    *) error "Unknown cli command: '$command'"; exit 1 ;;
  esac
}

cli_build() {
  local all=false
  [[ "${1:-}" == "--all" ]] && all=true

  header "Building rpicli"
  mkdir -p "$REPO_ROOT/bin"

  if $all; then
    for target in "linux/amd64" "linux/arm64"; do
      local goos="${target%/*}" goarch="${target#*/}"
      info "Building $goos/$goarch..."
      GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
        go build -ldflags="-s -w" \
          -o "$REPO_ROOT/bin/rpicli-${goos}-${goarch}" . 2>&1
      success "  bin/rpicli-${goos}-${goarch}"
    done
  else
    local goos goarch
    goos="$(go env GOOS)"
    goarch="$(go env GOARCH)"
    info "Building $goos/$goarch..."
    CGO_ENABLED=0 go build -ldflags="-s -w" \
      -o "$REPO_ROOT/bin/rpicli" . 2>&1
    success "  bin/rpicli"
  fi
}

cli_test() {
  header "Testing rpicli"
  go test -v -race -coverprofile="$CLI_DIR/coverage.out" ./... 2>&1
  go tool cover -func="$CLI_DIR/coverage.out"
}

cli_lint() {
  header "Linting rpicli"
  go vet ./... && success "go vet passed."
}

cli_install() {
  local dest="${1:-/usr/local/bin/rpicli}"
  header "Installing rpicli → $dest"
  cli_build
  install -m 0755 "$REPO_ROOT/bin/rpicli" "$dest"
  success "Installed to $dest"
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

main() {
  if [[ $# -eq 0 ]]; then usage; exit 1; fi

  local module="$1"; shift
  case "$module" in
    setup) cmd_setup "$@" ;;
    cli)   cd "$CLI_DIR" && cmd_cli "$@" ;;
    -h|--help) usage; exit 0 ;;
    *) error "Unknown module: '$module'"; usage; exit 1 ;;
  esac
}

main "$@"

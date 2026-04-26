#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

INVENTORY="homelab"
LIMIT=""
TAGS=""
SKIP_TAGS=""
VERBOSITY=""
CHECK_MODE="false"
VAULT_ARGS=()
EXTRA_VARS=()

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
${BOLD}rpi-cluster Ansible wrapper${RESET}

Usage: $(basename "$0") <command> [options]

${BOLD}Commands:${RESET}
  deps          Install required Ansible Galaxy collections
  ping          Run pre-flight hardware and connectivity validation (00-ping.yml)
  deploy        Bootstrap the full cluster (site.yml)
  reset         Tear down the cluster — DESTRUCTIVE, requires confirmation
  check         Dry-run deploy in --check --diff mode
  lint          Run yamllint and ansible-lint locally

${BOLD}Options:${RESET}
  -i, --inventory NAME       Inventory name under inventories/  (default: homelab)
  -l, --limit PATTERN        Limit execution to hosts matching pattern
  -t, --tags TAGS            Only run tasks tagged with these (comma-separated)
      --skip-tags TAGS       Skip tasks tagged with these (comma-separated)
  -e, --extra-vars KEY=VAL   Set extra variable (repeatable)
      --vault-pass-file FILE Use this vault password file
      --ask-vault-pass       Prompt for vault password
  -v, -vv, -vvv              Increase Ansible verbosity
  -h, --help                 Show this message

${BOLD}Examples:${RESET}
  $(basename "$0") deps
  $(basename "$0") ping
  $(basename "$0") deploy
  $(basename "$0") deploy -l rpi-0
  $(basename "$0") deploy -t common -vv
  $(basename "$0") deploy -e k3s_version=v1.32.3+k3s1
  $(basename "$0") check
  $(basename "$0") reset
  $(basename "$0") lint

EOF
}

require_value() {
  local opt="$1" val="${2:-}"
  if [[ -z "$val" ]]; then
    error "Option '$opt' requires a value."
    exit 1
  fi
}

parse_opts() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -i|--inventory)
        require_value "$1" "${2:-}"; INVENTORY="$2"; shift 2 ;;
      -l|--limit)
        require_value "$1" "${2:-}"; LIMIT="$2"; shift 2 ;;
      -t|--tags)
        require_value "$1" "${2:-}"; TAGS="$2"; shift 2 ;;
      --skip-tags)
        require_value "$1" "${2:-}"; SKIP_TAGS="$2"; shift 2 ;;
      -e|--extra-vars)
        require_value "$1" "${2:-}"; EXTRA_VARS+=("$2"); shift 2 ;;
      --vault-pass-file)
        require_value "$1" "${2:-}"
        VAULT_ARGS+=(--vault-password-file "$2"); shift 2 ;;
      --ask-vault-pass)
        VAULT_ARGS+=(--ask-vault-pass); shift ;;
      -v)   VERBOSITY="-v";   shift ;;
      -vv)  VERBOSITY="-vv";  shift ;;
      -vvv) VERBOSITY="-vvv"; shift ;;
      -h|--help)
        usage; exit 0 ;;
      *)
        error "Unknown option: '$1'"; usage; exit 1 ;;
    esac
  done
}

run_playbook() {
  local playbook="$1"
  shift

  local inventory_path="inventories/${INVENTORY}/hosts.yml"
  if [[ ! -f "$inventory_path" ]]; then
    error "Inventory not found: $inventory_path"
    exit 1
  fi

  local cmd=(
    ansible-playbook
    -i "$inventory_path"
    "$playbook"
  )

  [[ "$CHECK_MODE" == "true" ]] && cmd+=(--check --diff)
  [[ -n "$LIMIT" ]]             && cmd+=(-l "$LIMIT")
  [[ -n "$TAGS" ]]              && cmd+=(-t "$TAGS")
  [[ -n "$SKIP_TAGS" ]]         && cmd+=(--skip-tags "$SKIP_TAGS")
  [[ -n "$VERBOSITY" ]]         && cmd+=("$VERBOSITY")

  if [[ ${#VAULT_ARGS[@]} -gt 0 ]]; then
    cmd+=("${VAULT_ARGS[@]}")
  fi

  for var in "${EXTRA_VARS[@]+"${EXTRA_VARS[@]}"}"; do
    cmd+=(-e "$var")
  done

  cmd+=("$@")

  info "Running: ${cmd[*]}"
  echo
  "${cmd[@]}"
}

cmd_deps() {
  header "Installing Ansible Galaxy collections"
  ansible-galaxy collection install community.general ansible.posix
  success "Dependencies installed."
}

cmd_ping() {
  header "Pre-flight validation — inventory: ${INVENTORY}"
  run_playbook playbooks/00-ping.yml
}

cmd_deploy() {
  header "Deploying cluster — inventory: ${INVENTORY}"
  run_playbook site.yml
}

cmd_reset() {
  header "Cluster reset — inventory: ${INVENTORY}"
  warn "This will DESTROY the k3s cluster and wipe all node data."
  warn "Target inventory: ${INVENTORY}"
  echo
  read -r -p "  Type 'yes' to confirm: " confirm
  echo
  if [[ "$confirm" != "yes" ]]; then
    info "Aborted."
    exit 0
  fi
  run_playbook playbooks/99-reset.yml
}

cmd_check() {
  header "Dry-run (check + diff) — inventory: ${INVENTORY}"
  CHECK_MODE="true"
  run_playbook site.yml
}

cmd_lint() {
  header "Linting"
  local errors=0

  info "Running yamllint..."
  if yamllint .; then
    success "yamllint passed."
  else
    error "yamllint failed."
    errors=$((errors + 1))
  fi

  echo
  info "Running ansible-lint..."
  if ansible-lint; then
    success "ansible-lint passed."
  else
    error "ansible-lint failed."
    errors=$((errors + 1))
  fi

  echo
  if [[ $errors -gt 0 ]]; then
    error "$errors linter(s) reported issues."
    exit 1
  fi
  success "All linters passed."
}

main() {
  if [[ $# -eq 0 ]]; then
    usage
    exit 1
  fi

  local command="$1"
  shift

  parse_opts "$@"

  case "$command" in
    deps)      cmd_deps   ;;
    ping)      cmd_ping   ;;
    deploy)    cmd_deploy ;;
    reset)     cmd_reset  ;;
    check)     cmd_check  ;;
    lint)      cmd_lint   ;;
    -h|--help) usage; exit 0 ;;
    *)
      error "Unknown command: '$command'"
      usage
      exit 1
      ;;
  esac
}

main "$@"

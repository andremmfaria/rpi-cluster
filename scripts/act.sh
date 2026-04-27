#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

ACT_VERSION="v0.2.87"
ACT_PLATFORM="ubuntu-latest=catthehacker/ubuntu:act-22.04"
ACT_ARGS=()

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
${BOLD}rpi-cluster act wrapper — run GitHub Actions workflows locally${RESET}

Requires: act ${ACT_VERSION}+ (https://github.com/nektos/act)
          Docker (running)

Usage: $(basename "$0") <command> [options]

${BOLD}Commands:${RESET}
  install          Download and install act ${ACT_VERSION} to /usr/local/bin
  list             List all available workflows and jobs
  lint setup       Run cluster-setup lint   (yamllint + ansible-lint + syntax-check)
  lint platform    Run cluster-platform lint (manifests yamllint)
  lint all         Run all lint workflows
  all              Run all workflows
  run <name>       Run a specific workflow by filename (without .yml extension)

${BOLD}Options:${RESET}
  --secret-file FILE   Load secrets from FILE (act format: KEY=VALUE per line)
  --env-file FILE      Load env vars from FILE
  --dry-run            Print commands act would run without executing
  --privileged         Run containers with --privileged
  --reuse              Reuse existing containers instead of recreating
  -v, --verbose        Enable act verbose output
  -h, --help           Show this message

${BOLD}Examples:${RESET}
  $(basename "$0") install
  $(basename "$0") list
  $(basename "$0") lint setup
  $(basename "$0") lint platform
  $(basename "$0") lint all
  $(basename "$0") all
  $(basename "$0") run cluster-setup-lint

EOF
}

require_value() {
  local opt="$1" val="${2:-}"
  [[ -z "$val" ]] && { error "Option '$opt' requires a value."; exit 1; }
}

parse_opts() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --secret-file) require_value "$1" "${2:-}"; ACT_ARGS+=(--secret-file "$2"); shift 2 ;;
      --env-file)    require_value "$1" "${2:-}"; ACT_ARGS+=(--env-file "$2");    shift 2 ;;
      --dry-run)     ACT_ARGS+=(--dry-run); shift ;;
      --privileged)  ACT_ARGS+=(--container-options "--privileged"); shift ;;
      --reuse)       ACT_ARGS+=(--reuse); shift ;;
      -v|--verbose)  ACT_ARGS+=(-v); shift ;;
      -h|--help)     usage; exit 0 ;;
      *) error "Unknown option: '$1'"; usage; exit 1 ;;
    esac
  done
}

check_act() {
  command -v act &>/dev/null || {
    error "act is not installed. Run: $(basename "$0") install"
    exit 1
  }
}

check_docker() {
  docker info &>/dev/null 2>&1 || { error "Docker is not running."; exit 1; }
}

cmd_install() {
  header "Installing act ${ACT_VERSION}"
  local os arch bin_dir="/usr/local/bin" tmp
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
    x86_64)  arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) error "Unsupported architecture: $arch"; exit 1 ;;
  esac

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  local url="https://github.com/nektos/act/releases/download/${ACT_VERSION}/act_${os^}_${arch}.tar.gz"
  info "Downloading from: $url"
  curl -fsSL "$url" -o "${tmp}/act.tar.gz"
  tar -xzf "${tmp}/act.tar.gz" -C "$tmp" act
  install -m 0755 "${tmp}/act" "${bin_dir}/act"
  success "act installed to ${bin_dir}/act"
  act --version
}

run_act() {
  local event="$1" workflow_flag=(); shift
  [[ $# -gt 0 ]] && { workflow_flag=(-W "$1"); shift; }
  check_act
  check_docker
  act "$event" \
    -P "$ACT_PLATFORM" \
    "${workflow_flag[@]+"${workflow_flag[@]}"}" \
    "${ACT_ARGS[@]+"${ACT_ARGS[@]}"}" \
    "$@"
}

cmd_list() {
  check_act
  header "Available workflows and jobs"
  act -l -P "$ACT_PLATFORM" "${ACT_ARGS[@]+"${ACT_ARGS[@]}"}"
}

cmd_lint() {
  local target="${1:-}"; shift || true
  parse_opts "$@"
  case "$target" in
    setup)
      header "Running cluster-setup lint"
      run_act push ".github/workflows/cluster-setup-lint.yml"
      ;;
    platform)
      header "Running cluster-platform lint"
      run_act push ".github/workflows/cluster-platform-lint.yml"
      ;;
    all)
      header "Running all lint workflows"
      run_act push ".github/workflows/cluster-setup-lint.yml"
      echo
      run_act push ".github/workflows/cluster-platform-lint.yml"
      ;;
    *)
      error "lint requires: setup | platform | all"
      usage; exit 1
      ;;
  esac
}

cmd_all() {
  header "Running all workflows"
  parse_opts "$@"
  run_act push
}

cmd_run() {
  local name="${1:-}"; shift || true
  parse_opts "$@"
  [[ -z "$name" ]] && { error "No workflow name given."; usage; exit 1; }
  local path=".github/workflows/${name}.yml"
  [[ -f "$path" ]] || { error "Workflow not found: $path"; exit 1; }
  header "Running workflow: ${name}"
  run_act push "$path"
}

main() {
  if [[ $# -eq 0 ]]; then usage; exit 1; fi

  local command="$1"; shift

  case "$command" in
    install) parse_opts "$@"; cmd_install ;;
    list)    parse_opts "$@"; cmd_list    ;;
    lint)    cmd_lint "$@" ;;
    all)     cmd_all  "$@" ;;
    run)     cmd_run  "$@" ;;
    -h|--help) usage; exit 0 ;;
    *) error "Unknown command: '$command'"; usage; exit 1 ;;
  esac
}

main "$@"

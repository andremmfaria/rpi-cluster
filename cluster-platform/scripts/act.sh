#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# act searches for workflows relative to the repo root, not the subdirectory
REPO_ROOT="$(cd "$PROJECT_ROOT/.." && pwd)"

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
${BOLD}cluster-platform act wrapper — run GitHub Actions workflows locally${RESET}

Requires: act ${ACT_VERSION}+ (https://github.com/nektos/act)
          Docker (running)

Usage: $(basename "$0") <command> [options]

${BOLD}Commands:${RESET}
  install       Download and install act ${ACT_VERSION} to /usr/local/bin
  list          List all available workflows and jobs
  lint          Run the cluster-platform lint workflow
  all           Run all workflows
  run <name>    Run a specific workflow by filename (without .yml extension)

${BOLD}Options:${RESET}
  --secret-file FILE   Load secrets from FILE (act format: KEY=VALUE per line)
  --env-file FILE      Load env vars from FILE
  --dry-run            Print commands act would run without executing
  --reuse              Reuse existing containers instead of recreating them
  -v, --verbose        Enable act verbose output
  -h, --help           Show this message

${BOLD}Examples:${RESET}
  $(basename "$0") install
  $(basename "$0") lint
  $(basename "$0") all
  $(basename "$0") run lint

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
      --secret-file)
        require_value "$1" "${2:-}"
        ACT_ARGS+=(--secret-file "$2"); shift 2 ;;
      --env-file)
        require_value "$1" "${2:-}"
        ACT_ARGS+=(--env-file "$2"); shift 2 ;;
      --dry-run)
        ACT_ARGS+=(--dry-run); shift ;;
      --reuse)
        ACT_ARGS+=(--reuse); shift ;;
      -v|--verbose)
        ACT_ARGS+=(-v); shift ;;
      -h|--help)
        usage; exit 0 ;;
      *)
        error "Unknown option: '$1'"; usage; exit 1 ;;
    esac
  done
}

check_act() {
  if ! command -v act &>/dev/null; then
    error "act is not installed."
    echo
    echo "  Run: $(basename "$0") install"
    exit 1
  fi
}

check_docker() {
  if ! docker info &>/dev/null 2>&1; then
    error "Docker is not running or not accessible."
    exit 1
  fi
}

cmd_install() {
  header "Installing act ${ACT_VERSION}"

  local os arch bin_dir="/usr/local/bin" tmp
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$arch" in
    x86_64)  arch="amd64" ;;
    aarch64) arch="arm64" ;;
    arm64)   arch="arm64" ;;
    *)       error "Unsupported architecture: $arch"; exit 1 ;;
  esac

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  local url="https://github.com/nektos/act/releases/download/${ACT_VERSION}/act_${os^}_${arch}.tar.gz"
  local archive="${tmp}/act.tar.gz"

  info "Downloading from: $url"
  curl -fsSL "$url" -o "$archive"

  tar -xzf "$archive" -C "$tmp" act
  install -m 0755 "${tmp}/act" "${bin_dir}/act"

  success "act installed to ${bin_dir}/act"
  act --version
}

run_act() {
  local event="$1" workflow_flag=()
  shift

  if [[ $# -gt 0 ]]; then
    workflow_flag=(-W "$1")
    shift
  fi

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
  header "Running cluster-platform lint workflow"
  run_act push ".github/workflows/cluster-platform-lint.yml"
}

cmd_all() {
  header "Running cluster-platform workflows"
  run_act push ".github/workflows/cluster-platform-lint.yml"
}

cmd_run() {
  local name="${1:-}"
  if [[ -z "$name" ]]; then
    error "No workflow name given. Use 'list' to see available workflows."
    exit 1
  fi
  local path=".github/workflows/${name}.yml"
  if [[ ! -f "$path" ]]; then
    error "Workflow file not found: $path"
    exit 1
  fi
  header "Running workflow: ${name}"
  run_act push "$path"
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
    install) cmd_install ;;
    list)    cmd_list    ;;
    lint)    cmd_lint    ;;
    all)     cmd_all     ;;
    run)
      local target="${1:-}"
      shift || true
      parse_opts "$@"
      cmd_run "$target"
      ;;
    -h|--help) usage; exit 0 ;;
    *)
      error "Unknown command: '$command'"
      usage
      exit 1
      ;;
  esac
}

main "$@"

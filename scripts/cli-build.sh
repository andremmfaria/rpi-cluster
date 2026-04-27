#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
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
${BOLD}rpi-cluster build wrapper${RESET}

Usage: $(basename "$0") <command> [options]

${BOLD}Commands:${RESET}
  build [--all]    Build rpicli binary to ./bin/rpicli
                   --all  also builds linux/arm64
  test             Run unit tests with coverage
  lint             go vet
  install [path]   Build and install (default: /usr/local/bin/rpicli)

All cluster operations → use ./bin/rpicli

${BOLD}Examples:${RESET}
  $(basename "$0") build
  $(basename "$0") build --all
  $(basename "$0") test
  $(basename "$0") lint
  $(basename "$0") install

EOF
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

main() {
  if [[ $# -eq 0 ]]; then usage; exit 1; fi

  cd "$CLI_DIR"
  case "$1" in
    build)   shift; cli_build   "$@" ;;
    test)    cli_test              ;;
    lint)    cli_lint              ;;
    install) shift; cli_install "$@" ;;
    -h|--help) usage; exit 0 ;;
    *) error "Unknown command: '$1'"; usage; exit 1 ;;
  esac
}

main "$@"

#!/usr/bin/env bash
# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Apache License, Version 2.0.
#
# Elastic Docs Harness installer shim.
# Detects platform, downloads the correct binary from GitHub Releases,
# verifies the checksum, and runs the installer.
#
# Usage:
#   curl -sSL https://github.com/elastic/docs-harness/releases/latest/download/install.sh | bash
#   curl -sSL https://github.com/elastic/docs-harness/releases/latest/download/install.sh | bash -s -- --yes

set -euo pipefail

REPO="elastic/docs-harness"
RELEASES_BASE="https://github.com/${REPO}/releases"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="docs-harness"

# Colors (same conventions as elastic-docs-skills)
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${CYAN}${BOLD}ℹ${NC} $1"; }
ok()    { echo -e "${GREEN}${BOLD}✓${NC} $1"; }
warn()  { echo -e "${YELLOW}${BOLD}⚠${NC} $1"; }
err()   { echo -e "${RED}${BOLD}✗${NC} $1" >&2; }

# Detect OS and arch.
detect_platform() {
  local os arch

  case "$(uname -s)" in
    Darwin)  os="darwin" ;;
    Linux)   os="linux" ;;
    MINGW*|MSYS*|CYGWIN*) os="windows" ;;
    *) err "Unsupported OS: $(uname -s)"; exit 1 ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) err "Unsupported architecture: $(uname -m)"; exit 1 ;;
  esac

  # Windows arm64 binary not published; fall back to amd64.
  if [[ "$os" == "windows" && "$arch" == "arm64" ]]; then
    arch="amd64"
  fi

  echo "${os}_${arch}"
}

# Fetch the latest release tag from GitHub.
latest_version() {
  local tag
  tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [[ -z "$tag" ]] && { err "Could not fetch latest release version"; exit 1; }
  echo "$tag"
}

main() {
  local script_dir platform
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || pwd)"
  platform="$(detect_platform)"

  # ── Local-clone mode ─────────────────────────────────────────────────────
  # When running from a clone of the repo (private or not), prefer pre-built
  # binaries in bin/ over the network. No Go toolchain required.

  # 1. Platform-specific binary in bin/ (populated before going public).
  local platform_bin="$script_dir/bin/docs-harness_${platform}"
  if [[ -x "$platform_bin" ]]; then
    info "Using bundled binary: $platform_bin"
    "$platform_bin" "$@"
    return $?
  fi

  # 2. Binary built manually next to the script (developer workflow).
  if [[ -x "$script_dir/docs-harness" ]]; then
    info "Using local binary: $script_dir/docs-harness"
    "$script_dir/docs-harness" "$@"
    return $?
  fi

  # No local binary found — fall through to GitHub Releases download.
  # (This is the normal path once the repo is public.)
  info "Platform: ${platform}"

  local version
  version="$(latest_version)"
  info "Latest release: ${version}"

  local ext="tar.gz"
  [[ "$platform" == windows* ]] && ext="zip"

  local archive="${BINARY_NAME}_${platform}.${ext}"
  local download_url="${RELEASES_BASE}/download/${version}/${archive}"
  local checksum_url="${RELEASES_BASE}/download/${version}/checksums.txt"

  # Download to a temp directory.
  local tmpdir
  tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/docs-harness-XXXXXX")"
  trap "rm -rf '$tmpdir'" EXIT

  info "Downloading ${archive}..."
  curl -fsSL "$download_url" -o "${tmpdir}/${archive}"

  # Verify checksum.
  info "Verifying checksum..."
  local checksums
  checksums="$(curl -fsSL "$checksum_url")"
  local expected
  expected="$(echo "$checksums" | grep "$archive" | awk '{print $1}')"
  if [[ -z "$expected" ]]; then
    warn "Could not find checksum for ${archive} — skipping verification"
  else
    local actual
    actual="$(sha256sum "${tmpdir}/${archive}" 2>/dev/null | awk '{print $1}' \
               || shasum -a 256 "${tmpdir}/${archive}" | awk '{print $1}')"
    if [[ "$actual" != "$expected" ]]; then
      err "Checksum mismatch! Expected ${expected}, got ${actual}"
      exit 1
    fi
    ok "Checksum verified"
  fi

  # Extract binary.
  if [[ "$ext" == "zip" ]]; then
    unzip -q "${tmpdir}/${archive}" -d "${tmpdir}"
  else
    tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}"
  fi

  # Install binary.
  local binary_path="${tmpdir}/${BINARY_NAME}"
  [[ "$platform" == windows* ]] && binary_path="${binary_path}.exe"

  if [[ ! -x "$binary_path" ]]; then
    err "Binary not found after extraction: ${binary_path}"
    exit 1
  fi

  # Try to install to INSTALL_DIR; fall back to ~/.local/bin.
  local install_path="${INSTALL_DIR}/${BINARY_NAME}"
  if ! install -m 755 "$binary_path" "$install_path" 2>/dev/null; then
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "$INSTALL_DIR"
    install_path="${INSTALL_DIR}/${BINARY_NAME}"
    install -m 755 "$binary_path" "$install_path"
    warn "Installed to ${install_path} (not in /usr/local/bin — ensure ${INSTALL_DIR} is on your PATH)"
  fi
  ok "Installed docs-harness ${version} → ${install_path}"

  # Run the installer.
  echo ""
  "$install_path" "$@"
}

main "$@"

#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/brew-bump.sh vMAJOR.MINOR.PATCH

Update Homebrew formula URL + sha256 values for a GitHub release tag.
EOF
}

if [[ $# -ne 1 ]]; then
  usage
  exit 1
fi

version="$1"
if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$ ]]; then
  echo "Version must look like vMAJOR.MINOR.PATCH (received: ${version})." >&2
  exit 1
fi

tarball_url="https://github.com/organisciak/atmux/archive/refs/tags/${version}.tar.gz"

echo "Fetching ${tarball_url} for sha256..."
sha256="$(curl -fsSL "${tarball_url}" | shasum -a 256 | awk '{print $1}')"
if [[ -z "${sha256}" ]]; then
  echo "Failed to compute sha256 for ${tarball_url}." >&2
  exit 1
fi

update_formula() {
  formula_path="$1"
  if [[ ! -f "${formula_path}" ]]; then
    echo "Formula not found: ${formula_path}" >&2
    exit 1
  fi

  FORMULA_URL="${tarball_url}" FORMULA_SHA="${sha256}" perl -0777 -i -pe '
    s{^\s*url\s+".*?"$}{  url "$ENV{FORMULA_URL}"}m;
    s{^\s*sha256\s+".*?"$}{  sha256 "$ENV{FORMULA_SHA}"}m;
  ' "${formula_path}"
}

update_formula "homebrew/atmux.rb"
update_formula "homebrew/agent-tmux.rb"

echo "Updated formulae:"
echo "  - homebrew/atmux.rb"
echo "  - homebrew/agent-tmux.rb"
echo "Version: ${version}"
echo "sha256:  ${sha256}"

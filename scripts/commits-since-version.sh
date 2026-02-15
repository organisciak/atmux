#!/usr/bin/env bash
set -euo pipefail

mode="${1:-}"

latest_tag="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
if [[ -z "${latest_tag}" ]]; then
  case "${mode}" in
    --count-only) echo "0" ;;
    --tag-only) echo "" ;;
    --hook) echo "[atmux] Version reminder: no version tags found (expected vMAJOR.MINOR.PATCH)." ;;
    *) echo "No version tags found (expected vMAJOR.MINOR.PATCH)." ;;
  esac
  exit 0
fi

count="$(git rev-list --count "${latest_tag}..HEAD")"

case "${mode}" in
  --count-only)
    echo "${count}"
    ;;
  --tag-only)
    echo "${latest_tag}"
    ;;
  --hook)
    if [[ "${count}" -eq 0 ]]; then
      echo "[atmux] Version reminder: HEAD is at ${latest_tag} (0 commits since last bump)."
    else
      echo "[atmux] Version reminder: ${count} commit(s) since ${latest_tag}."
    fi
    ;;
  *)
    if [[ "${count}" -eq 0 ]]; then
      echo "HEAD is at ${latest_tag} (0 commits since last bump)."
    else
      echo "${count} commit(s) since last version bump ${latest_tag}."
    fi
    ;;
esac

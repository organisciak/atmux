#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/tag-version.sh vMAJOR.MINOR.PATCH [--push]

Create an annotated version tag whose message summarizes commits since the
previous version tag (v*).
EOF
}

if [[ $# -lt 1 ]]; then
  usage
  exit 1
fi

version="$1"
shift

push_tag=0
for arg in "$@"; do
  case "${arg}" in
    --push) push_tag=1 ;;
    *)
      echo "Unknown argument: ${arg}" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$ ]]; then
  echo "Version must look like vMAJOR.MINOR.PATCH (received: ${version})." >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/${version}" >/dev/null; then
  echo "Tag ${version} already exists." >&2
  exit 1
fi

previous_tag="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
if [[ -n "${previous_tag}" ]]; then
  range="${previous_tag}..HEAD"
  heading="Changes since ${previous_tag}:"
else
  range=""
  heading="Initial release changes:"
fi

if [[ -n "${range}" ]]; then
  changes="$(git log --no-merges --pretty=format:'- %s (%h)' "${range}")"
else
  changes="$(git log --no-merges --pretty=format:'- %s (%h)')"
fi

if [[ -z "${changes}" ]]; then
  changes="- No commits found since ${previous_tag:-repository start}."
fi

message_file="$(mktemp)"
trap 'rm -f "${message_file}"' EXIT

{
  echo "${version}"
  echo
  echo "${heading}"
  echo "${changes}"
} > "${message_file}"

git tag -a "${version}" -F "${message_file}"

echo "Created annotated tag ${version}."
if [[ -n "${previous_tag}" ]]; then
  echo "Compared against ${previous_tag}."
fi

if [[ "${push_tag}" -eq 1 ]]; then
  git push origin "${version}"
  echo "Pushed ${version} to origin."
fi

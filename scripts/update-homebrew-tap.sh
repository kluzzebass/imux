#!/usr/bin/env bash
# Update kluzzebass/homebrew-tap imux.rb from a published GitHub release's checksums.txt.
# Usage:
#   GITHUB_REPOSITORY=owner/repo HOMEBREW_TAP_TOKEN=... ./scripts/update-homebrew-tap.sh v0.2.0
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TAG="${1:?usage: $0 <tag e.g. v0.2.0>}"
REPO_SLUG="${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY to owner/repo (e.g. kluzzebass/imux)}"
TOKEN="${HOMEBREW_TAP_TOKEN:?set HOMEBREW_TAP_TOKEN}"

if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
  echo "expected tag like v0.2.0 or v1.0.0-rc1" >&2
  exit 1
fi

VERSION_NUM="${TAG#v}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "https://github.com/${REPO_SLUG}/releases/download/${TAG}/checksums.txt" -o "${TMP}/checksums.txt"

sha_of() {
  local name="$1"
  local h
  h="$(awk -v "n=${name}" '$NF == n { print $1; exit }' "${TMP}/checksums.txt")"
  if [[ -z "$h" ]]; then
    echo "checksum line not found for ${name}" >&2
    exit 1
  fi
  printf '%s' "$h"
}

SHA256_DARWIN_AMD64="$(sha_of imux-darwin-amd64)"
SHA256_DARWIN_ARM64="$(sha_of imux-darwin-arm64)"
SHA256_LINUX_AMD64="$(sha_of imux-linux-amd64)"
SHA256_LINUX_ARM64="$(sha_of imux-linux-arm64)"

git clone --depth 1 "https://x-access-token:${TOKEN}@github.com/kluzzebass/homebrew-tap.git" "${TMP}/tap"

cp "${ROOT}/packaging/homebrew/imux.rb.template" "${TMP}/tap/imux.rb"
sed -i.bak \
  -e "s|VERSION_NUM|${VERSION_NUM}|g" \
  -e "s|VERSION|${TAG}|g" \
  -e "s|SHA256_DARWIN_AMD64|${SHA256_DARWIN_AMD64}|g" \
  -e "s|SHA256_DARWIN_ARM64|${SHA256_DARWIN_ARM64}|g" \
  -e "s|SHA256_LINUX_AMD64|${SHA256_LINUX_AMD64}|g" \
  -e "s|SHA256_LINUX_ARM64|${SHA256_LINUX_ARM64}|g" \
  "${TMP}/tap/imux.rb"
rm -f "${TMP}/tap/imux.rb.bak"

(
  cd "${TMP}/tap"
  git config user.name "github-actions[bot]"
  git config user.email "github-actions[bot]@users.noreply.github.com"
  git add imux.rb
  if git diff --cached --quiet; then
    echo "homebrew-tap: imux.rb already up to date for ${TAG}"
    exit 0
  fi
  git commit -m "Update imux to ${TAG}"
  git push origin HEAD
)

echo "Updated homebrew-tap imux.rb for ${TAG}"

#!/usr/bin/env bash
# scripts/release/tag-release.sh
# Safely tags an ArkConstellation release candidate or final release only after Eng 3 gate check passes.

set -euo pipefail

TAG_NAME="${1:-}"

if [ -z "$TAG_NAME" ]; then
  echo "Usage: $0 <TAG_NAME>"
  echo "Example: $0 ark-v1.0.0-rc1"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Tag the repository this script lives in, not whatever directory the caller
# happened to be standing in.
cd "$REPO_ROOT"

if git remote get-url origin >/dev/null 2>&1; then
  echo ">>> Fetching tags from origin to check for an existing release..."
  git fetch origin --tags --quiet
fi

if git rev-parse -q --verify "refs/tags/$TAG_NAME" >/dev/null; then
  echo "!!! Error: tag '$TAG_NAME' already exists in $REPO_ROOT." >&2
  echo "    Refusing to overwrite a published tag. Delete it deliberately if that is intended." >&2
  exit 1
fi

echo ">>> Validating Eng 3 Security & Chaos Sign-Off for release tag: $TAG_NAME"
python3 "$SCRIPT_DIR/verify-eng3-signoff.py" "$TAG_NAME"

echo ">>> Creating annotated git tag '$TAG_NAME' in $REPO_ROOT..."
git tag -a "$TAG_NAME" -m "Release $TAG_NAME (Validated with Eng 3 Security & Chaos Sign-Off)"

echo ">>> Successfully created tag '$TAG_NAME'."
echo "    To push to remote: git push origin $TAG_NAME"

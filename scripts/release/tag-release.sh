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

echo ">>> Validating Eng 3 Security & Chaos Sign-Off for release tag: $TAG_NAME"
python3 "$SCRIPT_DIR/verify-eng3-signoff.py" "$TAG_NAME"

echo ">>> Creating annotated git tag '$TAG_NAME'..."
git tag -a "$TAG_NAME" -m "Release $TAG_NAME (Validated with Eng 3 Security & Chaos Sign-Off)"

echo ">>> Successfully created tag '$TAG_NAME'."
echo "    To push to remote: git push origin $TAG_NAME"

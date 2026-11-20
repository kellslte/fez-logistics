#!/usr/bin/env bash
# Usage: scripts/bump.sh patch|minor|major [--push]
# Runs tests, rewrites version.go, commits, tags vX.Y.Z (and pushes with --push).
set -euo pipefail
cd "$(dirname "$0")/.."
kind=${1:?usage: bump.sh patch|minor|major [--push]}
[ -z "$(git status --porcelain)" ] || { echo "working tree not clean" >&2; exit 1; }
cur=$(sed -n 's/^const Version = "\(.*\)"/\1/p' version.go)
IFS=. read -r ma mi pa <<<"$cur"
case $kind in
  major) ma=$((ma+1)); mi=0; pa=0 ;;
  minor) mi=$((mi+1)); pa=0 ;;
  patch) pa=$((pa+1)) ;;
  *) echo "kind must be patch|minor|major" >&2; exit 1 ;;
esac
new="$ma.$mi.$pa"
git rev-parse "v$new" >/dev/null 2>&1 && { echo "tag v$new exists" >&2; exit 1; }
go vet ./... && go test ./...
sed -i.bak "s/^const Version = .*/const Version = \"$new\"/" version.go && rm version.go.bak
git commit -qam "chore: release v$new"
git tag -a "v$new" -m "v$new"
echo "$cur -> $new (tagged v$new)"
if [ "${2:-}" = "--push" ]; then git push origin HEAD "v$new"; else echo "run: git push origin HEAD v$new"; fi

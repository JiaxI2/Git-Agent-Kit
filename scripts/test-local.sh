#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
REMOTE="$TMP/remote.git"
REPO="$TMP/repo"
WTROOT="$TMP/worktrees"
git init --bare "$REMOTE" >/dev/null
git clone "$REMOTE" "$REPO" >/dev/null 2>&1
cd "$REPO"
git config user.email gia-test@example.invalid
git config user.name gia-test
printf 'module example.com/sample\n\ngo 1.22\n' > go.mod
cat > main_test.go <<'EOT'
package sample
import "testing"
func TestOK(t *testing.T) {}
EOT
git add . && git commit -m 'init' >/dev/null
git branch -M main
git push -u origin main >/dev/null
"$ROOT/bin/gia" init --repo "$REPO" >/dev/null
git add .gia && git commit -m 'add gia config' >/dev/null && git push >/dev/null
BASE_BRANCH=$(git -C "$REPO" branch --show-current)
git -C "$REPO" branch agent/web-agent/feat/1-test origin/main
"$ROOT/bin/gia" worktree create --repo "$REPO" --ref agent/web-agent/feat/1-test --root "$WTROOT" > "$TMP/wt.json"
WT=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["data"]["path"])' "$TMP/wt.json")
HEAD=$(git -C "$WT" rev-parse HEAD)
test "$(git -C "$REPO" branch --show-current)" = "$BASE_BRANCH"
if "$ROOT/bin/gia" validate --repo "$WT" --profile smoke --expected-head deadbeef >/dev/null 2>&1; then echo 'expected SHA mismatch failure' >&2; exit 1; fi
"$ROOT/bin/gia" validate --repo "$WT" --profile smoke --expected-head "$HEAD" >/dev/null
printf dirty > "$WT/dirty.txt"
if "$ROOT/bin/gia" worktree remove --repo "$REPO" --path "$WT" >/dev/null 2>&1; then echo 'expected dirty worktree refusal' >&2; exit 1; fi
rm "$WT/dirty.txt"
"$ROOT/bin/gia" worktree remove --repo "$REPO" --path "$WT" >/dev/null
printf 'local integration test passed\n'

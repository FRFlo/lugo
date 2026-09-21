#!/usr/bin/env bash
# Contract tests for quality gates and the CI configuration that invokes them.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
gate="$root/scripts/quality-gate.sh"
workflow="$root/.github/workflows/test.yml"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
cat > "$tmp/bin/go" <<'SH'
#!/usr/bin/env bash
printf 'CGO_ENABLED=%s CC=%s args=%s\n' "${CGO_ENABLED:-}" "${CC:-}" "$*"
SH
chmod +x "$tmp/bin/go"

output=$(PATH="$tmp/bin:$PATH" bash "$gate" race)
grep -Fxq 'CGO_ENABLED=1 CC= args=test -count=1 -v -race ./...' <<<"$output"

if bash "$gate" invalid >"$tmp/invalid.out" 2>&1; then
  echo 'quality gate accepted an invalid mode' >&2
  exit 1
fi
grep -Fq 'usage: ' "$tmp/invalid.out"
grep -Fq 'race' "$tmp/invalid.out"

grep -Fq 'pull_request:' "$workflow"
grep -Fq "CGO_ENABLED: 1" "$workflow"
grep -Fq 'CC: gcc' "$workflow"
grep -Fq 'apt-get install --yes gcc' "$workflow"
grep -Fq "if: github.event_name == 'pull_request'" "$workflow"
grep -Fq '@vscode/vsce package' "$workflow"
if grep -Fq 'vsce publish' "$workflow"; then
  echo 'test workflow must not publish the VS Code extension' >&2
  exit 1
fi

echo 'Quality gate and CI configuration contract tests passed'

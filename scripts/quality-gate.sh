#!/usr/bin/env bash
# Repository quality and performance gates.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"

check_format() {
  local tmp file normalized
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' RETURN

  # Repository files currently use CRLF in some environments. Normalize line
  # endings before asking gofmt so the gate reports formatting, not EOL policy.
  while IFS= read -r -d '' file; do
    normalized="$tmp/${file//\//__}"
    tr -d '\r' < "$file" > "$normalized"
    local diff
    diff=$(gofmt -d "$normalized")
    if [[ -z "$diff" ]]; then
      continue
    fi
    echo "gofmt would change $file" >&2
    printf '%s\n' "$diff" >&2
    return 1
  done < <(git ls-files -z -- '*.go')

  git diff --check
}

check_coverage() {
  go test -cover ./...
}

check_benchmarks() {
  local output
  output=$(go test ./lexer ./parser -run '^$' -bench '^Benchmark(Lexer|Parser)$' -benchmem -count=1)
  printf '%s\n' "$output"

  awk '
    /^BenchmarkLexer-/ { lexer = 1; if ($(NF-1) != 0) { print "BenchmarkLexer allocates: " $(NF-1) " allocs/op" > "/dev/stderr"; bad = 1 } }
    /^BenchmarkParser-/ { parser = 1; if ($(NF-1) != 0) { print "BenchmarkParser allocates: " $(NF-1) " allocs/op" > "/dev/stderr"; bad = 1 } }
    END {
      if (!lexer) { print "BenchmarkLexer was not reported" > "/dev/stderr"; bad = 1 }
      if (!parser) { print "BenchmarkParser was not reported" > "/dev/stderr"; bad = 1 }
      exit bad
    }
  ' <<< "$output"
}

case "${1:-all}" in
  format) check_format ;;
  coverage) check_coverage ;;
  benchmark) check_benchmarks ;;
  all) check_format; check_coverage; check_benchmarks ;;
  *) echo "usage: $0 [all|format|coverage|benchmark]" >&2; exit 2 ;;
esac

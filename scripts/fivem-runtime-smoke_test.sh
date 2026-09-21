#!/usr/bin/env bash
# Contract tests for the optional runner. These tests are intentionally not
# called by quality-gate.sh: static tests must not depend on Python/curl or FXServer.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
runner="$root/scripts/fivem-runtime-smoke.sh"

skip_output=$(env -u FIVEM_RUNTIME_ENDPOINT -u FIVEM_SERVER_COMMAND bash "$runner")
grep -Fq 'SKIP: FiveM runtime tier: no FIVEM_RUNTIME_ENDPOINT or FIVEM_SERVER_COMMAND configured' <<<"$skip_output"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"; [[ -n "${server_pid:-}" ]] && kill "$server_pid" 2>/dev/null || true' EXIT
cat > "$tmp/server.py" <<'PY'
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({"manifest": True, "event": True, "export": True, "nui": False}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *_): pass
HTTPServer(("127.0.0.1", 30129), Handler).serve_forever()
PY
python3 "$tmp/server.py" & server_pid=$!
sleep 0.2
output=$(FIVEM_RUNTIME_ENDPOINT=http://127.0.0.1:30129 bash "$runner")
grep -Fq 'FiveM runtime smoke passed' <<<"$output"
echo 'FiveM runtime runner contract tests passed'

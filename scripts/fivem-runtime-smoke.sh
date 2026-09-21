#!/usr/bin/env bash
# Optional FiveM runtime smoke tier.
#
# The normal test suite never calls this script. Configure either
# FIVEM_RUNTIME_ENDPOINT (an already running server) or
# FIVEM_SERVER_COMMAND (a command which starts one). The command may use
# {resource} and {port} placeholders.
set -euo pipefail

endpoint=${FIVEM_RUNTIME_ENDPOINT:-}
command=${FIVEM_SERVER_COMMAND:-}
required=${FIVEM_RUNTIME_REQUIRED:-0}
resource=${FIVEM_RUNTIME_RESOURCE:-$(cd "$(dirname "$0")/fixtures/fivem-runtime-smoke" && pwd)}
port=${FIVEM_RUNTIME_PORT:-30120}
child_pid=

skip() {
  echo "SKIP: FiveM runtime tier: $1"
  exit 0
}
fail_or_skip() {
  if [[ "$required" == "1" ]]; then
    echo "ERROR: FiveM runtime tier: $1" >&2
    exit 1
  fi
  skip "$1 (set FIVEM_RUNTIME_REQUIRED=1 to make this an error)"
}

if [[ -z "$endpoint" && -z "$command" ]]; then
  skip "no FIVEM_RUNTIME_ENDPOINT or FIVEM_SERVER_COMMAND configured"
fi

if [[ -n "$command" ]]; then
  if [[ ! -d "$resource" ]]; then
    echo "ERROR: runtime smoke resource does not exist: $resource" >&2
    exit 1
  fi
  command=${command//\{resource\}/$resource}
  command=${command//\{port\}/$port}
  echo "Starting configured FiveM server command"
  bash -c "$command" &
  child_pid=$!
  trap 'kill "$child_pid" 2>/dev/null || true; wait "$child_pid" 2>/dev/null || true' EXIT
fi

if [[ -z "$endpoint" ]]; then
  endpoint="http://127.0.0.1:${port}"
fi
endpoint=${endpoint%/}
smoke_url=${FIVEM_RUNTIME_SMOKE_URL:-"$endpoint/lugo-runtime-smoke"}

if ! command -v curl >/dev/null 2>&1; then
  fail_or_skip "curl is not installed"
fi

# A server can take several seconds to load the resource. The endpoint is
# intentionally polled rather than sleeping for a fixed amount of time.
response=
for _ in $(seq 1 "${FIVEM_RUNTIME_ATTEMPTS:-30}"); do
  if response=$(curl --fail --silent --show-error --max-time 2 "$smoke_url" 2>/dev/null); then
    break
  fi
  response=
  sleep "${FIVEM_RUNTIME_INTERVAL:-1}"
done
if [[ -z "$response" ]]; then
  fail_or_skip "FiveM smoke endpoint is unavailable: $smoke_url"
fi

if ! command -v python3 >/dev/null 2>&1; then
  fail_or_skip "python3 is required to validate the smoke response"
fi
if ! SMOKE_RESPONSE="$response" python3 - <<'PY'
import json
import os
import sys

try:
    payload = json.loads(os.environ["SMOKE_RESPONSE"])
except (KeyError, json.JSONDecodeError) as exc:
    print(f"invalid smoke response: {exc}", file=sys.stderr)
    sys.exit(1)

required = {"manifest", "event", "export"}
missing = sorted(key for key in required if payload.get(key) is not True)
# NUI is optional because headless/server-only deployments cannot expose it.
if payload.get("nui") not in (True, False):
    missing.append("nui (must be true or false)")
if missing:
    print("smoke contract missing: " + ", ".join(missing), file=sys.stderr)
    sys.exit(1)
print("FiveM runtime smoke passed: manifest, event, export, NUI=" + str(payload["nui"]).lower())
PY
then
  echo "ERROR: FiveM smoke contract failed" >&2
  exit 1
fi

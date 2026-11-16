#!/usr/bin/env python3
"""Measure LSP round trips on an existing Lua workspace without changing its files."""
import argparse
import hashlib
import json
import os
import re
import select
import statistics
import subprocess
import sys
import tempfile
import time
from pathlib import Path


class RequestError(Exception):
    def __init__(self, method, error, elapsed_ms):
        super().__init__(f"{method}: {error}")
        self.error = error
        self.elapsed_ms = elapsed_ms


def safe_error(error):
    """Keep useful protocol status without retaining server supplied text."""
    code = error.get("code") if isinstance(error, dict) else None
    return {"code": code if isinstance(code, int) else "server_error", "message": "LSP request failed"}


KNOWN_NOTIFICATIONS = {
    "window/logMessage",
    "window/showMessage",
    "textDocument/publishDiagnostics",
    "$/progress",
    "$/logTrace",
}


def safe_notification_method(method):
    return method if isinstance(method, str) and method in KNOWN_NOTIFICATIONS else "other"


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--server", required=True, help="Lugo binary")
    ap.add_argument("--workspace", required=True)
    ap.add_argument("--file", required=True, help="Lua file inside workspace")
    ap.add_argument("--line", type=int, required=True, help="Zero-based symbol line")
    ap.add_argument("--character", type=int, required=True, help="Zero-based UTF-16 symbol column")
    ap.add_argument("--iterations", type=int, default=5)
    ap.add_argument("--query", default="WorkspaceMenu", help="Workspace symbol search query")
    ap.add_argument("--range-lines", type=int, default=60, help="Number of lines in the visible-range workload")
    ap.add_argument("--debug-export", action="store_true", help="Also request the potentially large debug export once")
    ap.add_argument("--timeout", type=float, default=120)
    args = ap.parse_args()
    if args.iterations < 1 or args.range_lines < 1:
        ap.error("iterations and range-lines must be positive")
    path = Path(args.file).resolve()
    root = Path(args.workspace).resolve()
    path.relative_to(root)
    source_bytes = path.read_bytes()
    source = source_bytes.decode("utf-8")
    errors = []
    repo = Path(__file__).resolve().parents[2]
    try:
        binary_hash = hashlib.sha256(Path(args.server).read_bytes()).hexdigest()
    except OSError:
        binary_hash = None
        errors.append("server binary SHA-256 unavailable")
    try:
        go_version = subprocess.run(["go", "version"], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True).stdout.strip()
    except (OSError, subprocess.CalledProcessError) as exc:
        go_version = None
        detail = f"exit status {exc.returncode}" if isinstance(exc, subprocess.CalledProcessError) else "Go executable unavailable"
        errors.append(f"Go version unavailable: {detail}")
    native_dir = repo / "lsp" / "stdlib"
    native_files = sorted(native_dir.glob("natives_*.lua"))
    native_digest = hashlib.sha256()
    if native_files:
        try:
            for native_file in native_files:
                native_digest.update(native_file.name.encode("utf-8") + b"\0")
                native_digest.update(native_file.read_bytes())
            native_stubs_hash = native_digest.hexdigest()
        except OSError as exc:
            native_stubs_hash = None
            errors.append("native stubs SHA-256 unavailable")
    else:
        native_stubs_hash = None
        errors.append("native stubs SHA-256 unavailable: no lsp/stdlib/natives_*.lua files found")
    uri = path.as_uri()
    doc = {"uri": uri}
    pos = {"line": args.line, "character": args.character}
    line_count = len(source.splitlines())
    full_range = {"start": {"line": 0, "character": 0}, "end": {"line": line_count, "character": 0}}
    visible_range = {"start": {"line": 0, "character": 0}, "end": {"line": min(line_count, args.range_lines), "character": 0}}
    empty_range = {"start": pos, "end": pos}
    env = dict(os.environ, LUGO_TELEMETRY="false")
    log = tempfile.TemporaryFile()
    proc = subprocess.Popen([args.server], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=log, env=env, bufsize=0)
    buffer = b""
    next_id = 0
    notifications = {}
    response_stats = (0, 0.0)

    def send(message):
        payload = json.dumps(message, separators=(",", ":"), ensure_ascii=False).encode()
        proc.stdin.write(b"Content-Length: " + str(len(payload)).encode() + b"\r\n\r\n" + payload)

    def recv(deadline):
        nonlocal buffer
        while True:
            sep = buffer.find(b"\r\n\r\n")
            if sep >= 0:
                match = re.search(br"(?im)^content-length:\s*(\d+)\s*$", buffer[:sep])
                if not match:
                    raise RuntimeError("invalid LSP header")
                length = int(match.group(1))
                if len(buffer) >= sep + 4 + length:
                    body = buffer[sep + 4:sep + 4 + length]
                    buffer = buffer[sep + 4 + length:]
                    decode_start = time.perf_counter()
                    message = json.loads(body)
                    return message, len(body), (time.perf_counter() - decode_start) * 1000
            remaining = deadline - time.monotonic()
            if remaining <= 0 or not select.select([proc.stdout], [], [], remaining)[0]:
                raise TimeoutError("LSP response timed out")
            data = os.read(proc.stdout.fileno(), 65536)
            if not data:
                raise RuntimeError(f"server exited: {proc.poll()}")
            buffer += data

    def notify(method, params):
        send({"jsonrpc": "2.0", "method": method, "params": params})

    def request(method, params):
        nonlocal next_id, response_stats
        next_id += 1
        request_id = next_id
        start = time.perf_counter()
        send({"jsonrpc": "2.0", "id": request_id, "method": method, "params": params})
        deadline = time.monotonic() + args.timeout
        while True:
            message, size, decode_ms = recv(deadline)
            if "method" in message:
                if "id" in message:
                    # Edits are measured but never applied to the user's workspace.
                    result = {"applied": False} if message["method"] == "workspace/applyEdit" else None
                    send({"jsonrpc": "2.0", "id": message["id"], "result": result})
                else:
                    key = safe_notification_method(message["method"])
                    notifications[key] = notifications.get(key, 0) + 1
                continue
            if message.get("id") == request_id:
                response_stats = (size, decode_ms)
                if "error" in message:
                    raise RequestError(method, message["error"], (time.perf_counter() - start) * 1000)
                return message.get("result"), (time.perf_counter() - start) * 1000

    results = {}
    def measure(name, method, params):
        samples = []
        sizes = []
        decode_times = []
        result = None
        errors = []
        for _ in range(args.iterations):
            try:
                result, ms = request(method, params)
            except RequestError as exc:
                result, ms = None, exc.elapsed_ms
            errors.append(safe_error(exc.error))
            samples.append(ms)
            sizes.append(response_stats[0])
            decode_times.append(response_stats[1])
        if isinstance(result, dict) and method.startswith("textDocument/semanticTokens/"):
            count = len(result.get("data", [])) // 5
        elif isinstance(result, dict) and method == "textDocument/completion":
            count = len(result.get("items", []))
        else:
            count = len(result) if isinstance(result, list) else (0 if result is None else 1)
        results[name] = {"median_ms": round(statistics.median(samples), 3), "max_ms": round(max(samples), 3), "samples_ms": [round(x, 3) for x in samples], "median_decode_ms": round(statistics.median(decode_times), 3), "response_bytes": sizes[-1], "result_count": count}
        if errors:
            results[name]["errors"] = errors
        return result

    try:
        _, init_ms = request("initialize", {"processId": os.getpid(), "rootUri": root.as_uri(), "capabilities": {"general": {"positionEncodings": ["utf-16"]}}})
        start = time.perf_counter()
        notify("initialized", {})
        # Requests are dispatched in order, so this response marks completion of indexing.
        request("workspace/symbol", {"query": args.query})
        index_ms = (time.perf_counter() - start) * 1000
        _, reindex_ms = request("lugo/reindex", {})
        start = time.perf_counter()
        notify("textDocument/didOpen", {"textDocument": {"uri": uri, "languageId": "lua", "version": 1, "text": source}})
        request("textDocument/documentSymbol", {"textDocument": doc})
        open_ms = (time.perf_counter() - start) * 1000
        at = {"textDocument": doc, "position": pos}
        for name in ("hover", "completion", "signatureHelp", "definition", "typeDefinition", "implementation", "documentHighlight", "prepareRename", "linkedEditingRange", "prepareCallHierarchy"):
            measure(name, "textDocument/" + name, at)
        measure("references", "textDocument/references", dict(at, context={"includeDeclaration": True}))
        measure("rename", "textDocument/rename", dict(at, newName="benchmarkName"))
        for name in ("documentSymbol", "foldingRange", "codeLens", "documentLink", "semanticTokens/full"):
            result = measure(name, "textDocument/" + name, {"textDocument": doc})
            if name == "codeLens": lenses = result
            if name == "documentLink": links = result
        measure("workspaceSymbol", "workspace/symbol", {"query": args.query})
        measure("willRenameFiles", "workspace/willRenameFiles", {"files": [{"oldUri": uri, "newUri": path.with_name("benchmark-renamed.lua").as_uri()}]})
        measure("applySafeFixes", "workspace/executeCommand", {"command": "lugo.applySafeFixes", "arguments": [uri]})
        measure("formatting", "textDocument/formatting", {"textDocument": doc, "options": {"tabSize": 4, "insertSpaces": False}})
        measure("rangeFormatting", "textDocument/rangeFormatting", {"textDocument": doc, "range": full_range, "options": {"tabSize": 4, "insertSpaces": False}})
        measure("rangeFormattingVisible", "textDocument/rangeFormatting", {"textDocument": doc, "range": visible_range, "options": {"tabSize": 4, "insertSpaces": False}})
        measure("selectionRange", "textDocument/selectionRange", {"textDocument": doc, "positions": [pos]})
        for name in ("inlayHint", "semanticTokens/range"):
            result = measure(name, "textDocument/" + name, {"textDocument": doc, "range": full_range})
            if name == "inlayHint": hints = result
        measure("semanticTokens/rangeVisible", "textDocument/semanticTokens/range", {"textDocument": doc, "range": visible_range})
        measure("inlayHintVisible", "textDocument/inlayHint", {"textDocument": doc, "range": visible_range})
        actions = measure("codeAction", "textDocument/codeAction", {"textDocument": doc, "range": empty_range, "context": {"diagnostics": []}})
        completions, _ = request("textDocument/completion", at)
        items = completions.get("items", []) if isinstance(completions, dict) else (completions or [])
        hierarchy, _ = request("textDocument/prepareCallHierarchy", at)
        for name, method, items2 in (("completionResolve", "completionItem/resolve", items), ("codeLensResolve", "codeLens/resolve", lenses or []), ("inlayHintResolve", "inlayHint/resolve", hints or []), ("documentLinkResolve", "documentLink/resolve", links or []), ("codeActionResolve", "codeAction/resolve", actions or [])):
            if items2:
                measure(name, method, items2[0])
            else:
                results[name] = {"skipped": "no item returned"}
        for name, method in (("incomingCalls", "callHierarchy/incomingCalls"), ("outgoingCalls", "callHierarchy/outgoingCalls")):
            if hierarchy:
                measure(name, method, {"item": hierarchy[0]})
            else:
                results[name] = {"skipped": "no call hierarchy item returned"}
        if args.debug_export:
            try:
                _, debug_ms = request("lugo/debugExport", {})
                results["debugExport"] = {"ms": round(debug_ms, 3), "response_bytes": response_stats[0], "decode_ms": round(response_stats[1], 3)}
            except RequestError as exc:
                results["debugExport"] = {"ms": round(exc.elapsed_ms, 3), "response_bytes": response_stats[0], "error": safe_error(exc.error)}
        start = time.perf_counter()
        notify("textDocument/didChange", {"textDocument": {"uri": uri, "version": 2}, "contentChanges": [{"text": source + "\n"}]})
        request("textDocument/documentSymbol", {"textDocument": doc})
        change_ms = (time.perf_counter() - start) * 1000
        start = time.perf_counter()
        notify("textDocument/didClose", {"textDocument": doc})
        request("workspace/symbol", {"query": args.query})
        close_ms = (time.perf_counter() - start) * 1000
        rss_kib = None
        try:
            for line in Path(f"/proc/{proc.pid}/status").read_text().splitlines():
                if line.startswith("VmHWM:"):
                    rss_kib = int(line.split()[1])
        except OSError:
            pass
        request("shutdown", None)
        notify("exit", None)
        proc.wait(timeout=10)
        print(json.dumps({"server_sha256": binary_hash, "position": pos, "initialize_ms": round(init_ms, 3), "index_ms": round(index_ms, 3), "reindex_ms": round(reindex_ms, 3), "open_ms": round(open_ms, 3), "change_ms": round(change_ms, 3), "close_ms": round(close_ms, 3), "peak_rss_kib": rss_kib, "notifications": notifications, "results": results, "metadata": {"go_version": go_version or "UNKNOWN", "native_stubs_sha256": native_stubs_hash or "UNKNOWN", "server_sha256": binary_hash or "UNKNOWN", "errors": errors}}, indent=2))
    finally:
        if proc.poll() is None:
            proc.kill()
            proc.wait()
        log.close()


if __name__ == "__main__":
    main()

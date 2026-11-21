#!/usr/bin/env python3
"""Run a command pinned to one available CPU and record wall time and peak RSS."""

import argparse
import json
import os
import pathlib
import subprocess
import sys
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stdout", required=True, help="capture child stdout here")
    parser.add_argument("--stderr", required=True, help="capture child stderr here")
    parser.add_argument("--metrics", required=True, help="write JSON metrics here")
    parser.add_argument(
        "--memory-limit", default="512MiB", help="soft Go runtime memory limit (default: 512MiB)"
    )
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not command:
        parser.error("provide a command after --")

    cpus = os.sched_getaffinity(0)
    cpu = min(cpus)
    env = os.environ.copy()
    env.update(GOMAXPROCS="1", GOMEMLIMIT=args.memory_limit, GOFLAGS="-p=1")
    start = time.perf_counter()
    peak_rss = 0
    with open(args.stdout, "wb") as stdout, open(args.stderr, "wb") as stderr:
        child = subprocess.Popen(
            command,
            stdout=stdout,
            stderr=stderr,
            env=env,
            preexec_fn=lambda: os.sched_setaffinity(0, {cpu}),
        )
        while child.poll() is None:
            try:
                resident_pages = int(pathlib.Path(f"/proc/{child.pid}/statm").read_text().split()[1])
                peak_rss = max(peak_rss, resident_pages * os.sysconf("SC_PAGE_SIZE"))
            except (FileNotFoundError, ProcessLookupError):
                pass
            time.sleep(0.02)

    metrics = {
        "exit_code": child.returncode,
        "wall_seconds": round(time.perf_counter() - start, 6),
        "sampled_peak_rss_bytes": peak_rss,
        "cpu_affinity": [cpu],
        "gomaxprocs": 1,
        "gomemlimit": f"{args.memory_limit} (soft)",
        "goflags": "-p=1",
        "command": command,
    }
    pathlib.Path(args.metrics).write_text(json.dumps(metrics, indent=2) + "\n")
    print(json.dumps(metrics, indent=2), file=sys.stderr)
    return child.returncode


if __name__ == "__main__":
    raise SystemExit(main())

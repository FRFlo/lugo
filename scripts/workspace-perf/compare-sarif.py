#!/usr/bin/env python3
"""Compare complete SARIF results as multisets, ignoring report ordering."""

import argparse
import collections
import json
import sys


def load_results(path):
    with open(path, encoding="utf-8") as source:
        report = json.load(source)
    return [
        json.dumps(result, sort_keys=True, separators=(",", ":"))
        for run in report.get("runs", [])
        for result in run.get("results", [])
    ]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("baseline", help="baseline SARIF report")
    parser.add_argument("candidate", help="candidate SARIF report")
    args = parser.parse_args()

    baseline = collections.Counter(load_results(args.baseline))
    candidate = collections.Counter(load_results(args.candidate))
    removed = baseline - candidate
    added = candidate - baseline
    print(
        f"baseline={sum(baseline.values())} candidate={sum(candidate.values())} "
        f"added={sum(added.values())} removed={sum(removed.values())}"
    )
    for label, delta in (("removed", removed), ("added", added)):
        for encoded, count in list(delta.items())[:10]:
            result = json.loads(encoded)
            uri = result.get("locations", [{}])[0].get("physicalLocation", {}).get("artifactLocation", {}).get("uri", "")
            region = result.get("locations", [{}])[0].get("physicalLocation", {}).get("region", {})
            print(f"{label} x{count}: {result.get('ruleId')} {uri}:{region.get('startLine')}:{region.get('startColumn')}")
    return 1 if added or removed else 0


if __name__ == "__main__":
    raise SystemExit(main())

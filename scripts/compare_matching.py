#!/usr/bin/env python3
"""Run identical replay/regression tests against a Git baseline and this tree.

No working-tree files are reverted. Both implementations run in temporary copies.
The report measures this curated corpus, not production accuracy or incident rates.
"""

import argparse
import collections
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


PATTERN = (
    r"^(TestCodex.*|TestScreen_SplitUTF8AndANSI|TestScreen_StreamEquivalence.*|"
    r"TestClaude_3OptionPrompt.*|TestCursor_.*|"
    r"TestProxy_E2E_(SelectionChangesDuringDelay|SeenPromptCancelsPreviousDelay|"
    r"ResizeInvalidatesDelayedMatch|FragmentedCodexApproval|CodexReappearance|CodexFooterRedraw))$"
)
PACKAGES = ["./internal/detector", "./internal/screen", "./internal/proxy"]


def run_tests(tree, output, env):
    with output.open("w") as log:
        result = subprocess.run(
            ["go", "test", "-json", "-count=1", "-run", PATTERN, *PACKAGES],
            cwd=tree, env=env, stdout=log, stderr=subprocess.STDOUT,
        )
    if result.returncode not in (0, 1):
        raise RuntimeError(f"Go test could not run; inspect {output}")
    events = []
    for line in output.read_text().splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    terminal = {
        (e["Package"], e["Test"]): e["Action"] for e in events
        if "Test" in e and e["Action"] in ("pass", "fail", "skip")
    }
    if not terminal:
        raise RuntimeError(f"No test results; inspect {output}")
    tested_packages = {package for package, _ in terminal}
    if len(tested_packages) != len(PACKAGES):
        raise RuntimeError(f"Not all selected packages ran tests; inspect {output}")
    # Count leaf cases only; parent subtest summaries must not inflate totals.
    leaves = {
        key: status for key, status in terminal.items()
        if not any(p == key[0] and t.startswith(key[1] + "/") for p, t in terminal)
    }
    return leaves


def origin(test):
    if test.startswith("TestCodex_UpstreamSnapshots/"):
        return "upstream_snapshot"
    if test.startswith("TestClaude_3OptionPrompt"):
        return "historical_regression"
    if test.startswith("TestCursor_"):
        return "existing_regression"
    return "constructed_regression"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", default="HEAD")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    sha = subprocess.check_output(
        ["git", "rev-parse", "--verify", args.baseline + "^{commit}"], cwd=repo, text=True
    ).strip()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="yoyo-matching-") as temp:
        temp = Path(temp)
        baseline, current = temp / "baseline", temp / "current"
        baseline.mkdir()
        current.mkdir()
        archive = temp / "baseline.tar"
        subprocess.run(["git", "archive", "--output", str(archive), sha], cwd=repo, check=True)
        subprocess.run(["tar", "-xf", str(archive), "-C", str(baseline)], check=True)
        for name in ("go.mod", "go.sum"):
            shutil.copy2(repo / name, current / name)
        # These packages contain all code/data used by the selected tests.
        shutil.copytree(repo / "internal", current / "internal")
        fingerprint = hashlib.sha256()
        for path in sorted(p for p in current.rglob("*") if p.is_file()):
            fingerprint.update(str(path.relative_to(current)).encode() + b"\0" + path.read_bytes())
        candidate_digest = fingerprint.hexdigest()
        for test in (current / "internal").rglob("*_test.go"):
            target = baseline / test.relative_to(current)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(test, target)
        shutil.copytree(
            current / "internal/detector/testdata", baseline / "internal/detector/testdata",
            dirs_exist_ok=True,
        )
        env = os.environ.copy()
        env.setdefault("GOCACHE", str(temp / "gocache"))
        old = run_tests(baseline, output / "baseline.jsonl", env)
        new = run_tests(current, output / "current.jsonl", env)
    if old.keys() != new.keys():
        raise RuntimeError("Test case sets differ; inspect logs before comparing")
    states = {("fail", "pass"): "fixed", ("pass", "pass"): "unchanged_pass",
              ("pass", "fail"): "regressed", ("fail", "fail"): "still_failing"}
    rows = [dict(package=p, test=t, origin=origin(t), baseline=old[p, t], current=new[p, t],
                 outcome=states.get((old[p, t], new[p, t]), "not_evaluated"))
            for p, t in sorted(old)]
    counts = dict(collections.Counter(row["outcome"] for row in rows))
    report = dict(baseline_commit=sha, candidate_sha256=candidate_digest,
                  comparison="working tree snapshot with identical current tests",
                  counting="leaf cases; widths and parameter variants are separate cases",
                  scope="curated regression corpus; raw production dump coverage not established",
                  test_pattern=PATTERN, counts=counts, cases=rows)
    (output / "comparison.json").write_text(json.dumps(report, indent=2) + "\n")
    lines = ["# Matching replay comparison", "", f"Baseline: `{sha}`. Candidate: current working-tree snapshot.",
             f"Candidate content SHA-256 (internal sources/data and go.mod/go.sum): `{candidate_digest}`.",
             "", "Both versions ran the same current tests. Counts are leaf cases, including width/parameter variants.",
             "This corpus is not a production incident sample. Its pass rate is not a production accuracy estimate.",
             "", "| Sample origin | Cases | Fixed | Preserved passes | Regressed | Still failing | Unevaluated |",
             "|---|---:|---:|---:|---:|---:|---:|"]
    for source in sorted({row["origin"] for row in rows}):
        group = [row for row in rows if row["origin"] == source]
        c = collections.Counter(row["outcome"] for row in group)
        lines.append(f"| {source} | {len(group)} | {c['fixed']} | {c['unchanged_pass']} | "
                     f"{c['regressed']} | {c['still_failing']} | {c['not_evaluated']} |")
    lines += ["", "| Case | Baseline | Candidate | Outcome |", "|---|---|---|---|"]
    lines += [f"| `{row['test']}` | {row['baseline']} | {row['current']} | {row['outcome']} |" for row in rows]
    (output / "comparison.md").write_text("\n".join(lines) + "\n")
    print(json.dumps(dict(baseline=sha, cases=len(rows), counts=counts, report=str(output / "comparison.md"))))
    return int(any(row["current"] != "pass" for row in rows))


if __name__ == "__main__":
    raise SystemExit(main())

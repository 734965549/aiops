"""Check production file sizes and prevent growth of existing exceptions."""

import argparse
import json
from pathlib import Path, PurePosixPath
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
BASELINE = "scripts/architecture_baseline.json"
FILE_LIMIT = 800
WEB_SUFFIXES = {".vue", ".ts", ".tsx", ".js", ".jsx", ".css", ".scss"}


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)


def is_production_source(name):
    path = PurePosixPath(name)
    if path.parts[:1] in {("cmd",), ("internal",), ("pkg",)}:
        return path.suffix == ".go" and not path.name.endswith("_test.go")
    return (
        path.parts[:2] == ("web", "src")
        and path.suffix in WEB_SUFFIXES
        and not {"__tests__", "__fixtures__"}.intersection(path.parts)
        and not path.name.endswith(".d.ts")
        and not path.stem.endswith((".test", ".spec"))
    )


def measure_sources():
    paths = git(
        "ls-files", "-z", "--cached", "--others", "--exclude-standard",
        "--", "cmd", "internal", "pkg", "web/src",
    ).decode("utf-8").split("\0")
    oversized = {}
    for name in sorted(set(paths) - {""}):
        path = ROOT / name
        if is_production_source(name) and path.is_file():
            lines = len(path.read_bytes().splitlines())
            if lines > FILE_LIMIT:
                oversized[name] = lines
    return oversized


def read_baseline(raw):
    baseline = json.loads(raw)
    if not isinstance(baseline, dict) or any(
        not is_production_source(name) or type(lines) is not int or lines <= FILE_LIMIT
        for name, lines in baseline.items()
    ):
        raise ValueError("Invalid architecture baseline")
    return baseline


def growth_errors(current, allowed):
    errors = []
    for name, lines in sorted(current.items()):
        limit = allowed.get(name, FILE_LIMIT)
        if lines > limit:
            errors.append(f"{name}: {lines} lines exceeds {limit}")
    return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tighten", action="store_true", help="Record only decreases and removals")
    parser.add_argument("--base-ref", help="Compare exceptions against this Git revision")
    args = parser.parse_args()

    try:
        path = ROOT / BASELINE
        allowed = read_baseline(path.read_bytes())
        current = measure_sources()
        errors = growth_errors(current, allowed)
        if args.base_ref:
            previous_files = git("ls-tree", "--name-only", args.base_ref, "--", BASELINE)
            previous = (
                read_baseline(git("show", f"{args.base_ref}:{BASELINE}"))
                if previous_files
                else measure_initial_baseline(args.base_ref)
            )
            errors.extend(f"Baseline expansion: {error}" for error in growth_errors(allowed, previous))
        if errors:
            print("\n".join(errors), file=sys.stderr)
            return 1
        if args.tighten and allowed != current:
            path.write_text(json.dumps(current, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
        elif allowed != current:
            print("Run python scripts/check_architecture.py --tighten to record reduced debt.", file=sys.stderr)
            return 1
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(error, file=sys.stderr)
        return 1

    print(f"Architecture file sizes passed ({len(current)} existing exceptions).")
    return 0


def measure_initial_baseline(ref):
    """During rollout, allow only the debt already present at the comparison ref."""
    paths = git("ls-tree", "-rz", "--name-only", ref, "--", "cmd", "internal", "pkg", "web/src")
    oversized = {}
    for name in paths.decode("utf-8").split("\0"):
        if name and is_production_source(name):
            lines = len(git("show", f"{ref}:{name}").splitlines())
            if lines > FILE_LIMIT:
                oversized[name] = lines
    return oversized


if __name__ == "__main__":
    sys.exit(main())

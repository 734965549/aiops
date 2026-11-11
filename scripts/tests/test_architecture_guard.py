import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


CHECKER = Path(__file__).resolve().parents[1] / "check_architecture.py"
BASELINE = "scripts/architecture_baseline.json"
LEGACY = "internal/example/legacy.go"


class ArchitectureGuardTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.git("init", "-q")
        (self.root / "scripts").mkdir()
        shutil.copyfile(CHECKER, self.root / "scripts" / CHECKER.name)
        self.write_source(LEGACY, 850)

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, text=True).strip()

    def commit(self):
        self.git("add", ".")
        self.git("-c", "user.name=Test", "-c", "user.email=test@example.com",
                 "commit", "-qm", "Baseline")
        return self.git("rev-parse", "HEAD")

    def write_source(self, name, lines):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("// example\n" * lines, encoding="utf-8")

    def write_baseline(self, entries):
        (self.root / BASELINE).write_text(json.dumps(entries), encoding="utf-8")

    def check(self, *args, exit_code=0):
        result = subprocess.run(
            [sys.executable, str(self.root / "scripts" / CHECKER.name), *args],
            cwd=self.root, capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, exit_code, result.stdout + result.stderr)
        return result.stderr

    def test_new_production_files_are_checked_and_tests_are_excluded(self):
        self.write_baseline({LEGACY: 850})
        self.write_source("internal/example/large_test.go", 900)
        self.write_source("web/src/example.spec.ts", 900)
        self.write_source("web/src/Example.vue", 800)
        self.check()
        self.write_source("web/src/Example.vue", 801)
        self.assertIn("web/src/Example.vue: 801 lines exceeds 800", self.check(exit_code=1))

    def test_legacy_growth_and_baseline_expansion_are_rejected(self):
        self.write_baseline({LEGACY: 850})
        base = self.commit()
        self.write_source(LEGACY, 851)
        self.assertIn("851 lines exceeds 850", self.check(exit_code=1))
        self.write_baseline({LEGACY: 851})
        self.assertIn("Baseline expansion", self.check("--base-ref", base, exit_code=1))

    def test_tighten_records_shrinks_and_removals(self):
        self.write_baseline({LEGACY: 850})
        base = self.commit()
        self.write_source(LEGACY, 820)
        self.check(exit_code=1)
        self.check("--tighten", "--base-ref", base)
        self.assertEqual(json.loads((self.root / BASELINE).read_text()), {LEGACY: 820})
        (self.root / LEGACY).unlink()
        self.check("--tighten", "--base-ref", base)
        self.assertEqual(json.loads((self.root / BASELINE).read_text()), {})

    def test_first_baseline_must_match_existing_debt(self):
        base = self.commit()
        self.write_baseline({LEGACY: 850})
        self.check("--base-ref", base)
        self.write_source("web/src/NewPage.vue", 801)
        self.write_baseline({LEGACY: 850, "web/src/NewPage.vue": 801})
        self.assertIn("Baseline expansion", self.check("--base-ref", base, exit_code=1))


if __name__ == "__main__":
    unittest.main()

"""Exercise EXIT cleanup from outside the repository without starting Docker."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().with_name("run.sh")


class CleanupTests(unittest.TestCase):
    def check_exit(self, fail_go, fail_cleanup, expected):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            log = root / "calls.jsonl"
            executable = f"#!{sys.executable}\n" + '''import json, os, sys
from pathlib import Path
name = Path(sys.argv[0]).name
with open(os.environ['GIFT_SCRIPT_LOG'], 'a') as out:
    out.write(json.dumps({'name': name, 'args': sys.argv[1:], 'cwd': os.getcwd()}) + '\\n')
if name == 'go' and os.environ['GIFT_FAIL_GO'] == '1': sys.exit(7)
if name == 'docker' and 'down' in sys.argv and os.environ['GIFT_FAIL_CLEANUP'] == '1': sys.exit(9)
'''
            for name in ("docker", "go", "node", "npm"):
                path = root / name
                path.write_text(executable)
                path.chmod(0o700)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                       GIFT_SCRIPT_LOG=str(log), GIFT_FAIL_GO=str(int(fail_go)),
                       GIFT_FAIL_CLEANUP=str(int(fail_cleanup)))
            result = subprocess.run(["bash", str(SCRIPT)], cwd=root, env=env, capture_output=True, text=True)
            self.assertEqual(expected, result.returncode, result.stderr)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            cleanup = [c for c in calls if c['name'] == 'docker' and 'down' in c['args']]
            self.assertEqual(1, len(cleanup))
            path = Path(cleanup[0]['args'][cleanup[0]['args'].index('-f') + 1])
            self.assertTrue(path.is_absolute())
            self.assertEqual(SCRIPT.with_name('compose.yaml'), path)
            if not fail_go:
                self.assertEqual('web', Path(cleanup[0]['cwd']).name)

    def test_success_cleans_after_cd_to_web(self):
        self.check_exit(False, False, 0)

    def test_failure_preserves_original_status(self):
        self.check_exit(True, False, 7)

    def test_cleanup_failure_fails_successful_run(self):
        self.check_exit(False, True, 1)

    def test_cleanup_failure_preserves_test_failure(self):
        self.check_exit(True, True, 7)


if __name__ == '__main__':
    unittest.main()

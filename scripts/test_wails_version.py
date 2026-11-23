"""Check the Wails version gate with stand-in CLIs, including output that keeps coming after the version line."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipIf(sys.platform == 'win32', 'uses POSIX shell stand-ins for the wails CLI')
class RequireWailsVersion(unittest.TestCase):
    def run_gate(self, script_body):
        with tempfile.TemporaryDirectory(prefix='testdesk-wails-gate-') as scratch:
            fake = Path(scratch) / 'wails'
            fake.write_text('#!/usr/bin/env bash\n' + script_body, encoding='utf-8', newline='\n')
            fake.chmod(0o755)
            environment = dict(os.environ, PATH=f'{scratch}{os.pathsep}{os.environ["PATH"]}')
            gate = (ROOT / 'scripts' / 'wails_version.sh').as_posix()
            command = f'set -euo pipefail; source "{gate}"; require_wails_version v2.17.0'
            return subprocess.run(['bash', '-c', command], env=environment, capture_output=True, text=True)

    def test_matching_version_passes_even_when_output_keeps_coming(self):
        result = self.run_gate('echo "Wails CLI v2.17.0"\nfor i in $(seq 1 3000); do echo "banner line $i"; done\n')
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_other_version_fails_with_the_install_command(self):
        result = self.run_gate('echo "Wails CLI v2.16.0"\n')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Install Wails v2.17.0', result.stderr)


if __name__ == '__main__':
    unittest.main()

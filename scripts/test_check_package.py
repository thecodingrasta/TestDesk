"""Regression tests for check_package.py when the temp directory is reached through a symlink.

macOS (/var -> /private/var) and Windows runners (8.3 RUNNER~1 paths) hand out temp
paths that differ from their resolved form. check_package.py once compared resolved
member paths with an unresolved root and rejected every healthy package there.
Linux with a symlinked TMPDIR reproduces the same path logic. The macOS branch of the
checker needs a real .app bundle, so this test runs on Linux only.
"""
import hashlib
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest

import check_package

SCRIPTS = Path(__file__).resolve().parent
VERSION = (SCRIPTS.parent / 'VERSION').read_text().strip()


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


@unittest.skipUnless(sys.platform.startswith('linux'), 'needs the Linux package layout and POSIX symlinks')
class CheckPackageThroughSymlinkedTemp(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory(prefix='testdesk-check-test-')
        self.addCleanup(self.scratch.cleanup)
        base = Path(self.scratch.name).resolve()
        stage, self.dist, real_tmp = base / 'stage', base / 'dist', base / 'real-tmp'
        self.link_tmp = base / 'link-tmp'
        for folder in (stage, real_tmp, stage / 'third_party' / 'licenses'):
            folder.mkdir(parents=True, exist_ok=True)
        self.link_tmp.symlink_to(real_tmp, target_is_directory=True)
        (stage / 'LICENSE').write_text('licence\n')
        (stage / 'THIRD_PARTY_NOTICES.md').write_text('notices\n')
        (stage / 'third_party' / 'licenses' / 'Go-LICENSE').write_text('go licence\n')
        for name in ('testdesk', 'testdesk-cli'):
            program = stage / name
            program.write_text(f'#!/bin/sh\necho {VERSION}\n')
            program.chmod(0o755)
        subprocess.run([sys.executable, str(SCRIPTS / 'package_release.py'), '--source', str(stage), '--output', str(self.dist)],
                       check=True, capture_output=True)

    def check(self):
        env = {**os.environ, 'TMPDIR': str(self.link_tmp)}
        return subprocess.run([sys.executable, str(SCRIPTS / 'check_package.py'), '--directory', str(self.dist)],
                              capture_output=True, text=True, env=env)

    def test_healthy_package_passes_when_temp_is_a_symlink(self):
        result = self.check()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_tampered_member_is_still_rejected_when_temp_is_a_symlink(self):
        archive = next(self.dist.glob('*.tar.gz'))
        unpacked = Path(self.scratch.name) / 'unpacked'
        unpacked.mkdir()
        with tarfile.open(archive) as packed:
            packed.extractall(unpacked, filter='data')
        (unpacked / 'LICENSE').write_text('changed\n')
        with tarfile.open(archive, 'w:gz') as repacked:
            for path in sorted(unpacked.iterdir()):
                repacked.add(path, arcname=path.name)
        sidecar = next(self.dist.glob('*.sha256'))
        sidecar.write_text(''.join(
            f'{sha256(self.dist / name) if name == archive.name else checksum}  {name}\n'
            for checksum, name in (line.split('  ', 1) for line in sidecar.read_text().splitlines())))
        result = self.check()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('package member checksum mismatch: LICENSE', result.stdout + result.stderr)


# Info.plist exactly as Wails v2.17.0 renders its embedded template for this project (name TestDesk,
# output file testdesk, default product fields). It starts at <!DOCTYPE with no <?xml?> declaration.
WAILS_INFO_PLIST = '''<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>CFBundlePackageType</key>
        <string>APPL</string>
        <key>CFBundleName</key>
        <string>TestDesk</string>
        <key>CFBundleExecutable</key>
        <string>testdesk</string>
        <key>CFBundleIdentifier</key>
        <string>com.wails.testdesk</string>
        <key>CFBundleVersion</key>
        <string>1.0.0</string>
        <key>CFBundleGetInfoString</key>
        <string>Built using Wails (https://wails.io)</string>
        <key>CFBundleShortVersionString</key>
        <string>1.0.0</string>
        <key>CFBundleIconFile</key>
        <string>iconfile</string>
        <key>LSMinimumSystemVersion</key>
        <string>10.13.0</string>
        <key>NSHighResolutionCapable</key>
        <string>true</string>
        <key>NSHumanReadableCopyright</key>
        <string>Copyright.........</string>
        
        
    </dict>
</plist>
'''


class MacBundleInspection(unittest.TestCase):
    """The macOS branch of the checker, exercised on any OS with a fake bundle."""

    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory(prefix='testdesk-bundle-test-')
        self.addCleanup(self.scratch.cleanup)
        self.app = Path(self.scratch.name) / 'TestDesk.app'
        (self.app / 'Contents' / 'MacOS').mkdir(parents=True)

    def write_plist(self, text):
        (self.app / 'Contents' / 'Info.plist').write_bytes(text.encode('utf-8'))

    def test_plist_as_wails_writes_it_is_read(self):
        self.write_plist(WAILS_INFO_PLIST)
        self.assertEqual(check_package.read_bundle_plist(self.app / 'Contents/Info.plist')['CFBundleExecutable'], 'testdesk')

    def test_plist_with_an_xml_declaration_is_read_unchanged(self):
        self.write_plist('<?xml version="1.0" encoding="UTF-8"?>\n' + WAILS_INFO_PLIST)
        self.assertEqual(check_package.read_bundle_plist(self.app / 'Contents/Info.plist')['CFBundleName'], 'TestDesk')

    def test_malformed_plist_fails_with_a_clear_message(self):
        for broken in ('', 'not a plist', WAILS_INFO_PLIST.replace('</dict>', '')):
            with self.subTest(broken=broken[:20]):
                self.write_plist(broken)
                with self.assertRaises(SystemExit) as raised:
                    check_package.read_bundle_plist(self.app / 'Contents/Info.plist')
                self.assertIn('not a valid property list', str(raised.exception))

    def test_bundle_executable_resolves_the_named_binary(self):
        self.write_plist(WAILS_INFO_PLIST)
        binary = self.app / 'Contents/MacOS/testdesk'
        binary.write_bytes(b'x')
        self.assertEqual(check_package.bundle_executable(self.app), binary)

    def test_bundle_executable_rejects_a_missing_or_unsafe_binary(self):
        self.write_plist(WAILS_INFO_PLIST)
        with self.assertRaises(SystemExit) as missing:
            check_package.bundle_executable(self.app)
        self.assertIn('missing its executable', str(missing.exception))
        self.write_plist(WAILS_INFO_PLIST.replace('<string>testdesk</string>', '<string>../escape</string>', 1))
        with self.assertRaises(SystemExit) as unsafe:
            check_package.bundle_executable(self.app)
        self.assertIn('CFBundleExecutable', str(unsafe.exception))


if __name__ == '__main__':
    unittest.main()

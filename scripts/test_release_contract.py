"""Exercise the release tag contract against disposable Git repositories, never the real tags or GitHub."""
import contextlib
import io
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import release_contract as contract


def git(repo, *args):
    env = {**os.environ, 'GIT_AUTHOR_NAME': 'Fixture', 'GIT_AUTHOR_EMAIL': 'fixture@example.invalid',
           'GIT_COMMITTER_NAME': 'Fixture', 'GIT_COMMITTER_EMAIL': 'fixture@example.invalid'}
    return subprocess.run(['git', '-C', str(repo), *args], check=True, capture_output=True, text=True, env=env).stdout.strip()


class VersionMappingTest(unittest.TestCase):
    """Golden table shared with tools/agentdist/version_test.go. Change both or neither."""

    GOLDEN = [
        ('1.21a', '1.21.1', '1.21.1', True),
        ('1.21b', '1.21.2', '1.21.2', True),
        ('1.21z', '1.21.26', '1.21.26', True),
        ('1.0.0', '1.0.0', '1.0.0', True),
        ('1.0.0-rc.1', '1.0.0-rc.1', '1.0.0', True),
    ]

    def test_golden_mappings(self):
        for release, manifest, bundle, valid in self.GOLDEN:
            with self.subTest(release=release):
                self.assertEqual(contract.is_release_version(release), valid)
                self.assertEqual(contract.manifest_version(release), manifest)
                self.assertEqual(contract.bundle_version(release), bundle)

    def test_malformed_versions_are_rejected(self):
        for value in ['', 'v1.21a', '1.21', '1.21A', '1.21aa', '1.21a1', '01.2.3', '1.21a-rc', '1.2.3-', '1.2.3 ', '1.21a;rm']:
            with self.subTest(value=value):
                self.assertFalse(contract.is_release_version(value))
                with self.assertRaises(contract.ReleaseError):
                    contract.manifest_version(value)
                with self.assertRaises(contract.ReleaseError):
                    contract.bundle_version(value)

    def test_prerelease_policy(self):
        self.assertFalse(contract.is_prerelease('1.21a'))
        self.assertFalse(contract.is_prerelease('1.0.0'))
        self.assertTrue(contract.is_prerelease('1.0.0-rc.1'))


class TagContractTest(unittest.TestCase):
    def setUp(self):
        scratch = tempfile.TemporaryDirectory(prefix='testdesk-release-contract-')
        self.addCleanup(scratch.cleanup)
        self.repo = Path(scratch.name) / 'repo'
        self.repo.mkdir()
        git(self.repo, 'init', '-q')
        git(self.repo, 'config', 'commit.gpgsign', 'false')
        self.write_version('1.0.0')
        git(self.repo, 'add', 'VERSION')
        git(self.repo, 'commit', '-q', '-m', 'release candidate')
        self.first = git(self.repo, 'rev-parse', 'HEAD')
        self.write_version('1.21a')
        git(self.repo, 'commit', '-q', '-am', 'bump to 1.21a')
        self.second = git(self.repo, 'rev-parse', 'HEAD')

    def write_version(self, text):
        (self.repo / 'VERSION').write_text(text + '\n', encoding='utf-8', newline='\n')

    def tag(self, name, commit, annotated=False):
        if annotated:
            git(self.repo, 'tag', '-a', '-m', name, name, commit)
        else:
            git(self.repo, 'tag', name, commit)

    def test_exact_lettered_tag_on_its_commit_verifies(self):
        self.tag('v1.21a', self.second)
        self.assertEqual(contract.verify_tag('v1.21a', self.second, self.repo), '1.21a')

    def test_annotated_tag_is_peeled_to_its_commit(self):
        self.tag('v1.21a', self.second, annotated=True)
        self.assertEqual(contract.verify_tag('v1.21a', self.second, self.repo), '1.21a')

    def test_tag_on_the_wrong_commit_is_rejected(self):
        self.tag('v1.21a', self.first)
        with self.assertRaisesRegex(contract.ReleaseError, 'points to'):
            contract.verify_tag('v1.21a', self.second, self.repo)

    def test_version_mismatch_is_rejected(self):
        self.tag('v1.0.0', self.first)
        with self.assertRaisesRegex(contract.ReleaseError, 'does not match VERSION'):
            contract.verify_tag('v1.0.0', self.first, self.repo)

    def test_missing_tag_is_rejected(self):
        with self.assertRaisesRegex(contract.ReleaseError, 'does not exist'):
            contract.verify_tag('v1.21a', self.second, self.repo)

    def test_malformed_input_is_rejected_before_git_runs(self):
        for tag in ['1.21a', 'v1.21', 'v1.21A', 'v01.2.3', 'v1.21a;rm', '-v1.21a', 'v1.0.0-']:
            with self.subTest(tag=tag):
                with self.assertRaisesRegex(contract.ReleaseError, 'must be'):
                    contract.verify_tag(tag, self.second, self.repo)

    def test_short_or_non_hex_commit_is_rejected(self):
        self.tag('v1.21a', self.second)
        for commit in [self.second[:12], self.second.upper(), 'HEAD']:
            with self.subTest(commit=commit):
                with self.assertRaisesRegex(contract.ReleaseError, '40-character'):
                    contract.verify_tag('v1.21a', commit, self.repo)

    def test_stable_and_prerelease_tags_still_verify(self):
        self.write_version('1.0.0')
        git(self.repo, 'commit', '-q', '-am', 'stable')
        stable = git(self.repo, 'rev-parse', 'HEAD')
        self.tag('v1.0.0', stable)
        self.assertEqual(contract.verify_tag('v1.0.0', stable, self.repo), '1.0.0')
        self.write_version('1.0.0-rc.1')
        git(self.repo, 'commit', '-q', '-am', 'rc')
        rc = git(self.repo, 'rev-parse', 'HEAD')
        self.tag('v1.0.0-rc.1', rc)
        self.assertEqual(contract.verify_tag('v1.0.0-rc.1', rc, self.repo), '1.0.0-rc.1')

    def test_draft_arguments_never_omit_draft(self):
        self.assertEqual(contract.draft_flags('v1.21a'), ['--verify-tag', '--draft', '--generate-notes'])
        self.assertEqual(contract.draft_flags('v1.0.0'), ['--verify-tag', '--draft', '--generate-notes'])
        self.assertEqual(contract.draft_flags('v1.0.0-rc.1'), ['--verify-tag', '--draft', '--generate-notes', '--prerelease'])
        for tag in ['1.21a', 'v1.21', 'v1.0.0;rm']:
            with self.subTest(tag=tag), self.assertRaises(contract.ReleaseError):
                contract.draft_flags(tag)

    def test_command_line_reports_failures_on_stderr(self):
        self.tag('v1.21a', self.first)
        errors = io.StringIO()
        with contextlib.redirect_stderr(errors):
            exit_code = contract.main(['verify', '--tag', 'v1.21a', '--commit', self.second, '--root', str(self.repo)])
        self.assertEqual(exit_code, 1)
        self.assertIn('points to', errors.getvalue())
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.assertEqual(contract.main(['draft-flags', '--tag', 'v1.21a']), 0)
        self.assertEqual(output.getvalue().split(), ['--verify-tag', '--draft', '--generate-notes'])


if __name__ == '__main__':
    unittest.main()

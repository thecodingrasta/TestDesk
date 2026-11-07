"""Tests for the retry and failure reporting in package.py's run_step."""
import contextlib
import io
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

import package


def python(code):
    return [sys.executable, '-c', code]


class RunStepTest(unittest.TestCase):
    def setUp(self):
        sleeper = mock.patch.object(package.time, 'sleep')
        self.sleep = sleeper.start()
        self.addCleanup(sleeper.stop)

    def run_step(self, command, **kwargs):
        errors = io.StringIO()
        with contextlib.redirect_stderr(errors):
            try:
                package.run_step('Fetching things', command, **kwargs)
            except SystemExit as exit_request:
                return str(exit_request.code), errors.getvalue()
        return None, errors.getvalue()

    def test_success_runs_once_without_waiting(self):
        outcome, errors = self.run_step(python('pass'), retries=3)
        self.assertIsNone(outcome)
        self.assertEqual(errors, '')
        self.sleep.assert_not_called()

    def test_transient_failure_is_retried_until_it_succeeds(self):
        with tempfile.TemporaryDirectory() as scratch:
            marker = Path(scratch) / 'first-attempt-done'
            code = ('import pathlib, sys; m = pathlib.Path(sys.argv[1]); ok = m.exists(); m.touch(); sys.exit(0 if ok else 3)')
            outcome, errors = self.run_step([*python(code), str(marker)], retries=3)
        self.assertIsNone(outcome)
        self.assertIn('Fetching things failed (attempt 1 of 3, exit code 3)', errors)
        self.assertEqual(self.sleep.call_count, 1)

    def test_persistent_failure_stops_after_the_last_attempt_and_names_the_step(self):
        outcome, errors = self.run_step(python('raise SystemExit(7)'), retries=3)
        self.assertEqual(outcome, 'Fetching things failed.')
        self.assertIn('attempt 3 of 3, exit code 7', errors)
        self.assertEqual(self.sleep.call_count, 2)

    def test_a_step_without_retries_fails_on_the_first_attempt(self):
        outcome, errors = self.run_step(python('raise SystemExit(1)'))
        self.assertEqual(outcome, 'Fetching things failed.')
        self.assertIn('attempt 1 of 1', errors)
        self.sleep.assert_not_called()


if __name__ == '__main__':
    unittest.main()

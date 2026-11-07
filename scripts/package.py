"""Build the native TestDesk package for this operating system and archive it with checksums.

CI candidate downloads and release drafts both call this script, so a download
is always produced by the same steps a contributor can run locally.
"""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]


ATTEMPTS = 3


def run_step(label, command, retries=1, **kwargs):
    """Run a command, retrying a network-dependent one, and name the step in the failure message.

    A bare CalledProcessError says which command died but not which phase of the package
    build it belonged to, which made one CI failure hard to attribute.
    """
    for attempt in range(1, retries + 1):
        try:
            subprocess.run(command, check=True, **kwargs)
            return
        except subprocess.CalledProcessError as error:
            print(f'{label} failed (attempt {attempt} of {retries}, exit code {error.returncode}): {" ".join(map(str, command))}',
                  file=sys.stderr, flush=True)
            if attempt == retries:
                sys.exit(f'{label} failed.')
            time.sleep(attempt * 5)


def go_output(*args):
    return subprocess.check_output(['go', *args], cwd=ROOT, text=True).strip()

def environment_with_wails():
    """Return an environment whose PATH has the Wails CLI matching go.mod, installing it when needed."""
    env = {**os.environ, 'TESTDESK_OUTPUT': ''}
    bin_dir = go_output('env', 'GOBIN') or str(Path(go_output('env', 'GOPATH')) / 'bin')
    env['PATH'] = bin_dir + os.pathsep + env['PATH']
    wanted = go_output('list', '-m', '-f', '{{.Version}}', 'github.com/wailsapp/wails/v2')
    # Resolve the full path: on Windows a bare name is looked up on the parent's PATH, not env's.
    wails = shutil.which('wails', path=env['PATH'])
    found = subprocess.run([wails, 'version'], capture_output=True, text=True, env=env).stdout if wails else ''
    if wanted not in found:
        run_step('Installing the Wails CLI', ['go', 'install', f'github.com/wailsapp/wails/v2/cmd/wails@{wanted}'],
                 retries=ATTEMPTS, cwd=ROOT, env=env)
    return env


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, default=ROOT / 'dist', help='directory that receives the archive and checksum')
    parser.add_argument('--stage', type=Path, default=ROOT / 'build' / 'stage', help='scratch directory for the unarchived build')
    args = parser.parse_args()
    stage = args.stage.resolve()
    output = args.output.resolve()
    shutil.rmtree(stage, ignore_errors=True)
    output.mkdir(parents=True, exist_ok=True)
    for stale in [*output.glob('TestDesk-*'), *output.glob('testdesk-cli-*')]:
        if stale.is_file():
            stale.unlink()
    env = environment_with_wails()
    env['TESTDESK_OUTPUT'] = str(stage)
    if sys.platform == 'win32':
        build = ['powershell', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File',
                 str(ROOT / 'scripts' / 'windows' / 'Build-TestDesk.ps1'), '-OutputDirectory', str(stage)]
    else:
        build = ['bash', str(ROOT / 'scripts' / 'build.sh')]
    run_step('Building the desktop app and CLI', build, cwd=ROOT, env=env)
    run_step('Archiving the package', [sys.executable, str(ROOT / 'scripts' / 'package_release.py'),
                                       '--source', str(stage), '--output', str(output)], cwd=ROOT, env=env)


if __name__ == '__main__':
    main()

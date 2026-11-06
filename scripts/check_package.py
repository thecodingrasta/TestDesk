"""Verify and exercise the downloads of one native build, using the bytes a user would get."""
import argparse
import hashlib
from pathlib import Path
import platform
import plistlib
import shutil
import subprocess
import sys
import tarfile
import tempfile
import zipfile
from xml.parsers.expat import ExpatError


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def make_executable(path):
    # Downloads arrive without the executable bit; the user's first step is to set it.
    path.chmod(path.stat().st_mode | 0o755)


def read_bundle_plist(path):
    """Parse a macOS Info.plist, accepting the form Wails writes.

    Wails renders Info.plist from a template that starts at <!DOCTYPE plist, with no
    <?xml ...?> declaration. macOS reads that, but plistlib rejects it as "Invalid file".
    Only a missing declaration is supplied here; any other malformed file still fails.
    """
    raw = path.read_bytes()
    if raw.lstrip().startswith((b'<!DOCTYPE', b'<plist')):
        raw = b'<?xml version="1.0" encoding="UTF-8"?>\n' + raw
    try:
        return plistlib.loads(raw)
    except (plistlib.InvalidFileException, ExpatError) as error:
        raise SystemExit(f'{path} is not a valid property list: {error}') from error


def bundle_executable(app):
    """Return the main executable named by a macOS app bundle's Info.plist."""
    bundle = read_bundle_plist(app / 'Contents/Info.plist')
    name = bundle.get('CFBundleExecutable')
    if not isinstance(name, str) or not name or Path(name).name != name:
        raise SystemExit(f'{app.name} has no usable CFBundleExecutable in Info.plist')
    executable = app / 'Contents/MacOS' / name
    if not executable.is_file():
        raise SystemExit(f'{app.name} is missing its executable Contents/MacOS/{name}')
    return executable


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--desktop-smoke', action='store_true')
    parser.add_argument('--directory', type=Path, default=Path('dist'))
    args = parser.parse_args()
    directory = args.directory
    sidecars = list(directory.glob('*.sha256'))
    if len(sidecars) != 1:
        parser.error('expected exactly one checksum file')
    listed = {}
    for line in sidecars[0].read_text().splitlines():
        checksum, name = line.split('  ', 1)
        if Path(name).name != name:
            raise SystemExit(f'unsafe download name: {name}')
        if not (directory / name).is_file() or digest(directory / name) != checksum:
            raise SystemExit(f'download checksum mismatch: {name}')
        listed[name] = checksum
    unexpected = {path.name for path in directory.iterdir()} - set(listed) - {sidecars[0].name}
    if unexpected:
        raise SystemExit(f'files are not covered by the checksum file: {sorted(unexpected)}')
    archives = [name for name in listed if name.endswith(('.zip', '.tar.gz'))]
    if len(archives) != 1:
        parser.error('expected exactly one native archive')
    archive = directory / archives[0]
    bare_cli = [name for name in listed if name.startswith('testdesk-cli-')]
    bare_desktop = [name for name in listed if name.startswith('TestDesk-') and name != archives[0]]
    if len(bare_cli) != 1:
        raise SystemExit('expected exactly one standalone CLI download')
    if len(bare_desktop) != (0 if sys.platform == 'darwin' else 1):
        raise SystemExit('unexpected standalone desktop downloads')
    with tempfile.TemporaryDirectory(prefix='testdesk-package-') as scratch:
        # Resolve the root once: the containment checks below compare resolved member paths against it, and
        # temp directories are often reached through a link (macOS /var -> /private/var, Windows 8.3 RUNNER~1).
        root = (Path(scratch) / 'package').resolve()
        root.mkdir()
        if archive.suffix == '.zip':
            with zipfile.ZipFile(archive) as packed:
                for name in packed.namelist():
                    if not (root / name).resolve().is_relative_to(root):
                        raise SystemExit('unsafe ZIP member')
                packed.extractall(root)
        else:
            with tarfile.open(archive) as packed:
                packed.extractall(root, filter='data')
        manifest = root / 'SHA256SUMS'
        for line in manifest.read_text().splitlines():
            checksum, name = line.split('  ', 1)
            path = (root / name).resolve()
            if not path.is_relative_to(root):
                raise SystemExit(f'package checksum entry escapes the package: {name}')
            if not path.is_file() or digest(path) != checksum:
                raise SystemExit(f'package member checksum mismatch: {name}')
        if not (root / 'third_party/licenses/Go-LICENSE').is_file():
            raise SystemExit('runtime licence texts are missing')
        suffix = '.exe' if sys.platform == 'win32' else ''
        cli = root / f'testdesk-cli{suffix}'
        version = (Path(__file__).resolve().parents[1] / 'VERSION').read_text().strip()
        if subprocess.check_output([str(cli), 'version'], text=True).strip() != version:
            raise SystemExit('packaged CLI version mismatch')
        if sys.platform == 'darwin':
            apps = list(root.glob('*.app'))
            if len(apps) != 1:
                raise SystemExit('expected one macOS app bundle')
            executable = bundle_executable(apps[0])
            arch = subprocess.check_output(['lipo', '-archs', str(executable)], text=True).strip()
            if platform.machine() not in arch.split():
                raise SystemExit(f'package does not contain native architecture {platform.machine()}')
        else:
            choices = [root / f'TestDesk{suffix}', root / f'testdesk{suffix}']
            executable = next((path for path in choices if path.is_file()), None)
            if executable is None:
                raise SystemExit('desktop binary is missing')
        if subprocess.check_output([str(executable), 'version'], text=True).strip() != version:
            raise SystemExit('packaged desktop version mismatch')

        # The standalone downloads must be the packaged bytes and must run with nothing beside them.
        alone = Path(scratch) / 'on-path'
        alone.mkdir()
        if digest(directory / bare_cli[0]) != digest(cli):
            raise SystemExit('standalone CLI differs from the packaged CLI')
        shutil.copyfile(directory / bare_cli[0], alone / f'testdesk-cli{suffix}')
        make_executable(alone / f'testdesk-cli{suffix}')
        if subprocess.check_output([str(alone / f'testdesk-cli{suffix}'), 'version'], text=True, cwd=alone).strip() != version:
            raise SystemExit('standalone CLI version mismatch')
        if bare_desktop:
            if digest(directory / bare_desktop[0]) != digest(executable):
                raise SystemExit('standalone desktop differs from the packaged desktop')
            standalone = alone / f'testdesk{suffix}'
            shutil.copyfile(directory / bare_desktop[0], standalone)
            make_executable(standalone)
            if subprocess.check_output([str(standalone), 'version'], text=True, cwd=alone).strip() != version:
                raise SystemExit('standalone desktop version mismatch')
            executable = standalone
        if args.desktop_smoke:
            if bare_desktop:
                subprocess.run([sys.executable, str(Path(__file__).with_name('cli_pair_smoke.py')), str(alone)], check=True)
            subprocess.run([sys.executable, str(Path(__file__).with_name('desktop_smoke.py')), str(executable)], check=True)
        print(f'Extracted package and standalone download checks passed: {archive.name}')


if __name__ == '__main__':
    main()

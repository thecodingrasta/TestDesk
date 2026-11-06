"""Archive a native build, and publish its executables on their own, with checksums.

Every target gets a full package (executables plus licence texts) and the same
executables as plain files, so a user can download one file, put it on PATH and run it.
macOS ships its desktop app only inside the package, because an app is a bundle
directory and cannot be a single file.
"""
import argparse
import hashlib
from pathlib import Path
import platform
import shutil
import tarfile
import zipfile


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', type=Path, default=Path('build/stage'))
    parser.add_argument('--output', type=Path, default=Path('dist'))
    args = parser.parse_args()
    if not args.source.is_dir():
        parser.error('native build directory is missing')
    required = [args.source / 'LICENSE', args.source / 'THIRD_PARTY_NOTICES.md',
                args.source / 'third_party' / 'licenses']
    if not all(path.exists() for path in required):
        parser.error('include LICENSE, THIRD_PARTY_NOTICES.md and third_party/licenses before packaging')
    source = args.source.resolve()
    destination = args.output.resolve()
    if destination == source or destination.is_relative_to(source):
        parser.error('output must be outside the build directory')
    members = sorted(path for path in source.rglob('*') if path.is_file() and path.name != 'SHA256SUMS')
    if not any(path.name in {'TestDesk.exe', 'testdesk.exe', 'testdesk'} for path in members):
        parser.error('desktop executable is missing')
    checksums = ''.join(f'{sha256(path)}  {path.relative_to(source).as_posix()}\n' for path in members)
    manifest = source / 'SHA256SUMS'
    manifest.write_text(checksums, encoding='utf-8', newline='\n')
    members.append(manifest)
    version = (Path(__file__).resolve().parents[1] / 'VERSION').read_text().strip()
    arch = {'x86_64': 'amd64', 'AMD64': 'amd64', 'aarch64': 'arm64'}.get(platform.machine(), platform.machine().lower())
    system = {'Darwin': 'macos'}.get(platform.system(), platform.system().lower())
    suffix = '.exe' if system == 'windows' else ''
    base = f'TestDesk-{version}-{system}-{arch}'
    destination.mkdir(parents=True, exist_ok=True)
    if system == 'windows':
        archive = destination / f'{base}.zip'
        with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED) as output:
            for path in members:
                output.write(path, path.relative_to(source))
    else:
        archive = destination / f'{base}.tar.gz'
        with tarfile.open(archive, 'w:gz') as output:
            for path in sorted(source.iterdir()):
                output.add(path, arcname=path.name)
    published = [archive]
    cli = destination / f'testdesk-cli-{version}-{system}-{arch}{suffix}'
    shutil.copyfile(source / f'testdesk-cli{suffix}', cli)
    published.append(cli)
    if system != 'macos':
        desktop = destination / f'{base}{suffix}'
        executable = next(path for path in members if path.parent == source and path.name in {'TestDesk.exe', 'testdesk'})
        shutil.copyfile(executable, desktop)
        published.append(desktop)
    # One checksum file per target lists every download, so a single `sha256sum -c` verifies them all.
    (destination / f'{base}.sha256').write_text(
        ''.join(f'{sha256(path)}  {path.name}\n' for path in published), encoding='utf-8', newline='\n')
    for path in published:
        print(path)


if __name__ == '__main__':
    main()

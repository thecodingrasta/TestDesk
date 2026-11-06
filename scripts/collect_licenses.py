"""Collect runtime module licence texts for Windows, Linux and macOS packages."""
import json
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def objects(data):
    decoder = json.JSONDecoder()
    while data.strip():
        data = data.lstrip()
        value, end = decoder.raw_decode(data)
        yield value
        data = data[end:]


def main():
    modules = {}
    for platform, arch in [('windows', 'amd64'), ('linux', 'amd64'),
                           ('darwin', 'amd64'), ('darwin', 'arm64')]:
        env = dict(os.environ, GOOS=platform, GOARCH=arch, CGO_ENABLED='1')
        # Listing does not compile native sources. -e allows foreign-platform
        # CGo prerequisites to be absent while retaining module provenance.
        result = subprocess.check_output(
            ['go', 'list', '-e', '-tags=production,webkit2_41', '-deps', '-json', './', './cmd/testdesk'],
            cwd=ROOT, env=env, text=True, encoding='utf-8')
        for package in objects(result):
            module = package.get('Module', {})
            if module and not module.get('Main'):
                modules[module['Path']] = module
    destination = ROOT / 'third_party' / 'licenses'
    destination.mkdir(parents=True, exist_ok=True)
    # A dependency bump can drop a module; its licence text must not ship as if it were still used.
    current = {name.replace('/', '__') for name in modules}
    for stale in destination.iterdir():
        if stale.is_dir() and stale.name not in current:
            shutil.rmtree(stale)
    rows = []
    for name, module in sorted(modules.items()):
        source = Path(module['Dir'])
        texts = sorted(p for p in source.iterdir() if p.is_file() and
                       (p.name.upper().startswith(('LICENSE', 'NOTICE', 'COPYING')) or
                        p.name.upper() in {'AUTHORS', 'PATENTS'}))
        if not any(p.name.upper().startswith(('LICENSE', 'COPYING')) for p in texts):
            raise RuntimeError(f'No licence found for {name}')
        folder = name.replace('/', '__')
        target = destination / folder
        target.mkdir(exist_ok=True)
        links = []
        for path in texts:
            shutil.copyfile(path, target / path.name)
            links.append(f'[{path.name}](third_party/licenses/{folder}/{path.name})')
        rows.append(f"| `{name}` | `{module['Version']}` | {', '.join(links)} |")
    go_root = Path(subprocess.check_output(['go', 'env', 'GOROOT'], cwd=ROOT, text=True).strip())
    shutil.copyfile(go_root / 'LICENSE', destination / 'Go-LICENSE')
    notice = '''# Third-party notices

TestDesk is licensed under GPL-3.0. Runtime dependencies retain their own licences below. These texts are copied from the module versions selected by Go; regenerate with `python scripts/collect_licenses.py` after dependency changes. Review upstream notices when changing dependencies or packaging.

The inventory is the union of runtime imports listed for Windows amd64, Linux amd64 and macOS amd64/arm64. It excludes build-only tools and CI provider fixtures. Native system libraries and platform WebViews are installed separately. TestDesk does not bundle Chrome or Chromium. Wails embeds its frontend runtime under its MIT licence. The Go runtime uses the [Go licence](third_party/licenses/Go-LICENSE).

| Module | Version | Licence and notices |
|---|---|---|
'''
    (ROOT / 'THIRD_PARTY_NOTICES.md').write_text(notice + '\n'.join(rows) + '\n', encoding='utf-8', newline='\n')
    print(f'Collected licence texts for {len(modules)} runtime modules and Go.')


if __name__ == '__main__':
    main()

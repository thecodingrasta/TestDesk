"""Reject mismatched versions before packaging a release.

VERSION holds the exact release text (for example 1.21a). Every other copy must agree with it,
or with the stricter form that the mapping in release_contract.py derives for it.
"""
import json
from pathlib import Path
import re
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
import release_contract  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]


def read_constant(name, constant):
    source = (ROOT / name).read_text(encoding='utf-8')
    match = re.search(rf'const {constant} = "([^"]+)"', source)
    return match[1] if match else None


def main():
    expected = (ROOT / 'VERSION').read_text(encoding='utf-8').strip()
    if not release_contract.is_release_version(expected):
        raise SystemExit('VERSION must contain a release version without a v prefix, such as 1.0.0, 1.0.0-rc.1 or 1.21a')
    for name, constant in [('main.go', 'desktopVersion'), ('cmd/testdesk/main.go', 'version'),
                           ('internal/mcp/server.go', 'serverVersion')]:
        if read_constant(name, constant) != expected:
            raise SystemExit(f'{name}: version differs from VERSION')
    distribution = ROOT / 'agent-distribution/testdesk'
    agentdist = json.loads((distribution / 'agentdist.json').read_text(encoding='utf-8'))
    if agentdist.get('defaultVersion') != expected:
        raise SystemExit('agent-distribution/testdesk/agentdist.json: defaultVersion differs from VERSION')
    manifest = release_contract.manifest_version(expected)
    for relative in ['plugin/plugin.json', 'adapters/claude-code/.claude-plugin/plugin.json',
                     'adapters/cursor/.cursor-plugin/plugin.json']:
        found = json.loads((distribution / relative).read_text(encoding='utf-8')).get('version')
        if found != manifest:
            raise SystemExit(f'agent-distribution/testdesk/{relative}: version {found!r} differs from {manifest!r}')
    wails = json.loads((ROOT / 'wails.json').read_text(encoding='utf-8'))
    bundle = release_contract.bundle_version(expected)
    if wails.get('info', {}).get('productVersion') != bundle:
        raise SystemExit(f'wails.json: info.productVersion differs from {bundle!r} (bundle form of {expected})')
    print(f'Version strings agree: {expected} (manifest {manifest}, bundle {bundle})')


if __name__ == '__main__':
    main()

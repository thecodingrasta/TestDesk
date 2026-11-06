"""Reject mismatched versions before packaging a release."""
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def main():
    expected = (ROOT / 'VERSION').read_text().strip()
    if not re.fullmatch(r'\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?', expected):
        raise SystemExit('VERSION must contain a release version without a v prefix')
    for name, constant in [('main.go', 'desktopVersion'), ('cmd/testdesk/main.go', 'version'),
                           ('internal/mcp/server.go', 'serverVersion')]:
        source = (ROOT / name).read_text(encoding='utf-8')
        match = re.search(rf'const {constant} = "([^"]+)"', source)
        if not match or match[1] != expected:
            raise SystemExit(f'{name}: version differs from VERSION')
    print(f'Version strings agree: {expected}')


if __name__ == '__main__':
    main()

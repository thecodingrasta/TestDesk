"""Check publishable paths, local documentation links and skill metadata."""
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]


def git_paths(*args):
    data = subprocess.check_output(['git', *args, '-z'], cwd=ROOT)
    return {name for name in data.decode('utf-8').split('\0') if name}


def heading_slugs(markdown_path):
    content = re.sub(r'```.*?```', '', markdown_path.read_text(encoding='utf-8-sig'), flags=re.S)
    counts = {}
    slugs = set()
    for match in re.finditer(r'^#{1,6}\s+(.+?)\s*#*\s*$', content, flags=re.M):
        raw = re.sub(r'`([^`]*)`', r'\1', match[1])
        raw = re.sub(r'\[([^\]]+)\]\([^)]+\)', r'\1', raw)
        slug = re.sub(r'[^a-z0-9\s-]', '', raw.lower()).strip()
        slug = re.sub(r'\s', '-', slug)
        idx = counts.get(slug, 0)
        counts[slug] = idx + 1
        slugs.add(slug if idx == 0 else f'{slug}-{idx}')
    return slugs


def main():
    tracked = git_paths('ls-files', '--cached')
    candidates = tracked | git_paths('ls-files', '--others', '--exclude-standard')
    errors = []
    slug_cache = {}
    blocked = re.compile(r'(^|/)(\.cache|\.codex|\.gitnexus|\.testdesk|node_modules|test-results|coverage|__pycache__|ijfw)(/|$)|^(dist/|docs/design/|docs/launch/)|\.(exe|dll|log|tmp|pyc)$|\.testdesk-backup-')
    for name in sorted(tracked):
        if blocked.search(name):
            errors.append(f'generated/private path is tracked: {name}')
    for name in sorted(candidates):
        path = ROOT / name
        if not path.is_file() or blocked.search(name):
            continue
        if path.suffix.lower() not in {'.md', '.mdc'}:
            continue
        content = path.read_text(encoding='utf-8-sig')
        if path.name == 'SKILL.md':
            frontmatter = re.match(r'\A---\r?\n(.*?)\r?\n---', content, re.S)
            if not frontmatter or not all(re.search(rf'^{key}:\s*\S', frontmatter[1], re.M) for key in ['name', 'description']):
                errors.append(f'missing skill metadata: {name}')
        # Ignore code samples; validate Markdown links, anchors, and HTML image sources.
        prose = re.sub(r'```.*?```', '', content, flags=re.S)
        links = re.findall(r'!?\[[^\]]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)', prose)
        links += re.findall(r'<img\b[^>]*\bsrc="([^"]+)"', prose)
        for link in links:
            parsed = urlsplit(link.strip('<>'))
            if parsed.scheme or parsed.netloc:
                continue
            if not parsed.path and parsed.fragment:
                target = path.resolve()
            elif parsed.path:
                target = (path.parent / unquote(parsed.path)).resolve()
            else:
                continue
            if not target.is_relative_to(ROOT) or not target.exists():
                errors.append(f'broken local link in {name}: {link}')
                continue
            if parsed.fragment and target.is_file() and target.suffix.lower() == '.md':
                if target not in slug_cache:
                    slug_cache[target] = heading_slugs(target)
                if parsed.fragment not in slug_cache[target]:
                    errors.append(f'broken anchor in {name}: {link}')
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print('Repository paths, documentation links and skill metadata passed.')
    return 0


if __name__ == '__main__':
    sys.exit(main())

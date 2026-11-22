"""Release tag, version and draft contract shared by the release workflow and the local checks.

The external tag is v<VERSION>, and VERSION keeps the exact tag text (for example 1.21a).
Plugin manifests and native bundles need stricter syntax, so this module derives them. The same
mapping lives in tools/agentdist/version.go; the golden tables in both test suites keep them aligned.
A single letter is a patch revision (a is patch 1), and docs/RELEASING.md records the policy.
"""
import argparse
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

_SEMVER = r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?'
_LETTERED = r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)([a-z])'
SEMVER_RELEASE = re.compile(_SEMVER)
LETTERED_RELEASE = re.compile(_LETTERED)
COMMIT = re.compile(r'[0-9a-f]{40}')
MAX_LENGTH = 64


class ReleaseError(Exception):
    """A tag, version or commit does not satisfy the release contract."""


def is_release_version(value):
    return len(value) <= MAX_LENGTH and bool(SEMVER_RELEASE.fullmatch(value) or LETTERED_RELEASE.fullmatch(value))


def letter_patch(letter):
    """Map a single lowercase letter to its patch number: a is 1, z is 26."""
    return ord(letter) - ord('a') + 1


def manifest_version(value):
    """Return the semantic version that plugin manifests carry, such as 1.21.1 for 1.21a."""
    lettered = LETTERED_RELEASE.fullmatch(value)
    if lettered:
        return f'{lettered[1]}.{lettered[2]}.{letter_patch(lettered[3])}'
    if is_release_version(value) and SEMVER_RELEASE.fullmatch(value):
        return value
    raise ReleaseError(f'release version {value!r} cannot be mapped to a plugin manifest version')


def bundle_version(value):
    """Return the numeric version that Info.plist and Windows version resources carry, such as 1.21.1."""
    lettered = LETTERED_RELEASE.fullmatch(value)
    if lettered:
        return f'{lettered[1]}.{lettered[2]}.{letter_patch(lettered[3])}'
    semver = SEMVER_RELEASE.fullmatch(value)
    if semver and len(value) <= MAX_LENGTH:
        return f'{semver[1]}.{semver[2]}.{semver[3]}'
    raise ReleaseError(f'release version {value!r} cannot be mapped to a bundle version')


def is_prerelease(value):
    """Only hyphenated semantic versions are prereleases. A letter is a patch revision."""
    return SEMVER_RELEASE.fullmatch(value) is not None and '-' in value


def version_from_tag(tag):
    if not tag.startswith('v') or not is_release_version(tag[1:]):
        raise ReleaseError(f'tag {tag!r} must be v<MAJOR>.<MINOR>.<PATCH>[-prerelease] or v<MAJOR>.<MINOR><letter>')
    return tag[1:]


def verify_tag(tag, commit, root=ROOT):
    """Require the tag to match VERSION and to name exactly the commit being built."""
    version = version_from_tag(tag)
    if not COMMIT.fullmatch(commit):
        raise ReleaseError('the commit to build must be a full 40-character lowercase SHA')
    recorded = (Path(root) / 'VERSION').read_text(encoding='utf-8').strip()
    if recorded != version:
        raise ReleaseError(f'tag {tag} does not match VERSION {recorded!r}')
    try:
        resolved = subprocess.run(
            ['git', '-C', str(root), 'rev-parse', '--verify', '--end-of-options', f'refs/tags/{tag}^{{commit}}'],
            capture_output=True, text=True, check=True).stdout.strip()
    except subprocess.CalledProcessError as error:
        raise ReleaseError(f'tag {tag} does not exist in this repository') from error
    if resolved != commit:
        raise ReleaseError(f'tag {tag} points to {resolved}, not the commit being built ({commit})')
    return version


def draft_flags(tag):
    """Return the flags for `gh release create`. A draft is always created; nothing publishes automatically."""
    version = version_from_tag(tag)
    flags = ['--verify-tag', '--draft', '--generate-notes']
    if is_prerelease(version):
        flags.append('--prerelease')
    return flags


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    commands = parser.add_subparsers(dest='command', required=True)
    verify = commands.add_parser('verify', help='check the tag, VERSION and commit')
    verify.add_argument('--tag', required=True)
    verify.add_argument('--commit', required=True)
    verify.add_argument('--root', type=Path, default=ROOT, help=argparse.SUPPRESS)
    flags = commands.add_parser('draft-flags', help='print the gh release create flags, one per line')
    flags.add_argument('--tag', required=True)
    args = parser.parse_args(argv)
    try:
        if args.command == 'verify':
            version = verify_tag(args.tag, args.commit, args.root)
            print(f'Verified {args.tag} for version {version} at {args.commit}.')
        else:
            print('\n'.join(draft_flags(args.tag)))
    except ReleaseError as error:
        print(f'release contract: {error}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())

"""Prepare disposable CI fixtures for opt-in real-provider tests."""
import argparse
from pathlib import Path
import shutil
import subprocess


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', required=True, type=Path)
    parser.add_argument('--npm-cli', type=Path, help='optional path to npm-cli.js for an isolated npm installation')
    args = parser.parse_args()
    root = args.root.resolve()
    root.mkdir(parents=True, exist_ok=False)
    fixture = Path(__file__).resolve().parent / 'fixtures/providers'
    node = root / 'node'
    shutil.copytree(fixture, node, ignore=shutil.ignore_patterns('node_modules'))
    npm = shutil.which('npm.cmd') or shutil.which('npm')
    if not npm:
        raise SystemExit('npm is required for the provider test fixtures')
    npm_command = ['node', str(args.npm_cli.resolve())] if args.npm_cli else [npm]
    subprocess.run([*npm_command, 'ci', '--ignore-scripts', '--no-fund', '--no-audit'], cwd=node, check=True)
    subprocess.run([*npm_command, 'audit', '--audit-level=moderate'], cwd=node, check=True)
    packages = root / 'dotnet'
    packages.mkdir()
    (packages / 'fixture.csproj').write_text('''<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup><TargetFrameworks>net8.0;net10.0</TargetFrameworks></PropertyGroup>
  <ItemGroup>
    <PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1" />
    <PackageReference Include="xunit" Version="2.9.3" />
    <PackageReference Include="xunit.runner.visualstudio" Version="3.1.4" />
    <PackageReference Include="NUnit" Version="4.1.0" />
    <PackageReference Include="NUnit3TestAdapter" Version="4.5.0" />
  </ItemGroup>
</Project>
''', encoding='utf-8')
    # Existing integration tests deliberately restore from this offline feed.
    subprocess.run(['dotnet', 'restore', str(packages / 'fixture.csproj'), '--packages',
                    str(Path.home() / '.nuget/packages'), '--source',
                    'https://api.nuget.org/v3/index.json'], check=True)
    print(f'Provider fixtures prepared in {root}')


if __name__ == '__main__':
    main()

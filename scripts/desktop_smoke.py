#!/usr/bin/env python3
"""Exercise a packaged desktop, tray readiness and real MCP runs in isolation."""

import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time


class Client:
    def __init__(self, connection, stream):
        self.connection = connection
        self.stream = stream
        self.sequence = 0
        self.request('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {},
                                   'clientInfo': {'name': 'ci-smoke', 'version': '1'}})
        self.stream.write(b'{"jsonrpc":"2.0","method":"notifications/initialized"}\n')

    def request(self, method, params):
        self.sequence += 1
        self.connection.settimeout(45)
        self.stream.write((json.dumps({'jsonrpc': '2.0', 'id': self.sequence,
                                      'method': method, 'params': params}) + '\n').encode())
        while True:
            line = self.stream.readline(1024 * 1024)
            if not line or not line.endswith(b'\n'):
                raise RuntimeError('desktop closed the session or returned an oversized response')
            response = json.loads(line)
            if response.get('id') != self.sequence:
                continue
            if 'error' in response:
                raise RuntimeError(f'MCP request failed: {response["error"]}')
            return response['result']

    def tool(self, name, **arguments):
        result = self.request('tools/call', {'name': name, 'arguments': arguments})
        if result.get('isError'):
            raise RuntimeError(f'{name} failed: {result}')
        return json.loads(result['content'][0]['text'])

    def close(self):
        self.stream.close()
        self.connection.close()


def fixture(root, name):
    project = root / name
    project.mkdir()
    command = {'exe': sys.executable, 'args': ['-c',
        'import sys,time; name=sys.argv[1]; time.sleep(30 if name=="slow" else 0); sys.exit(1 if name=="fail" else 0)',
        '{{test.selector}}'], 'timeoutSeconds': 40}
    config = {'version': 1, 'name': name, 'workspace': '.testdesk', 'providers': [{
        'id': 'smoke', 'name': 'Smoke', 'kind': 'custom', 'root': '.', 'enabled': True,
        'discover': {'parser': 'lines', 'command': {'exe': sys.executable,
            'args': ['-c', 'print("pass\\nfail\\nslow")'], 'timeoutSeconds': 10}},
        'run': {'all': command, 'test': command}, 'results': {'format': 'none'}}]}
    (project / 'testdesk.json').write_text(json.dumps(config), encoding='utf-8')
    return str(project)


def exercise(client, first, second):
    client.tool('testdesk_capabilities')
    client.tool('testdesk_project_add', projectRoot=second, activate=False)
    for project in (first, second):
        discovery = client.tool('testdesk_discover', project=project)
        if discovery.get('warning') or discovery['count'] != 3:
            raise RuntimeError(f'packaged discovery failed: {discovery}')
        catalog = client.tool('testdesk_tests', project=project, limit=10)
        ids = {item['name']: item['id'] for item in catalog['tests']}
        cases = [('pass', 'passed'), ('fail', 'failed'), ('slow', 'cancelled')] if project == first else [('pass', 'passed')]
        for name, expected in cases:
            selection = {'project': project, 'testIds': [ids[name]]}
            if client.tool('testdesk_plan', **selection)['total'] != 1:
                raise RuntimeError('exact selection did not produce one work item')
            run = client.tool('testdesk_run', **selection)
            if name == 'slow':
                deadline = time.monotonic() + 10
                while not client.tool('testdesk_snapshot', project=project).get('running'):
                    if time.monotonic() >= deadline:
                        raise RuntimeError('cancellation fixture never started')
                    time.sleep(0.1)
                client.tool('testdesk_control', project=project, action='stop')
            if not client.tool('testdesk_wait', project=project, timeoutMs=30000)['idle']:
                raise RuntimeError('packaged run failed to become idle')
            details = client.tool('testdesk_diagnostics', project=project, runId=run['runId'], maxBytes=4096)
            if details['run']['status'] != expected:
                raise RuntimeError(f'expected {expected} for {name}: {details["run"]}')
    if client.tool('testdesk_snapshot', project=first)['project'] != Path(first).name:
        raise RuntimeError('multi-project selection leaked between projects')


def open_mcp(lease_path, expected_pid):
    """Return a ready socket only for the desktop started by this check."""
    try:
        lease = json.loads(lease_path.read_text(encoding="utf-8"))
        if lease["pid"] != expected_pid:
            raise RuntimeError("the lifecycle lease belongs to another process")
        host, port = lease["address"].rsplit(":", 1)
        connection = socket.create_connection((host, int(port)), timeout=1)
        connection.settimeout(1)
        stream = connection.makefile("rwb", buffering=0)
        stream.write((lease["token"] + "\n").encode())
        if stream.readline(1024) != b"ok\n":
            raise OSError("desktop authentication failed")
        stream.write(b"mcp-v1\n")
        if stream.readline(1024) != b"ready\n":
            raise OSError("desktop MCP session is not ready")
        return connection, stream
    except (FileNotFoundError, json.JSONDecodeError, KeyError, OSError, ValueError):
        if "stream" in locals():
            stream.close()
        if "connection" in locals():
            connection.close()
        return None


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: desktop_smoke.py <packaged executable>")
    executable = Path(sys.argv[1]).resolve(strict=True)
    with tempfile.TemporaryDirectory(prefix="testdesk-desktop-smoke-") as directory:
        config_dir = Path(directory) / "config"
        environment = os.environ.copy()
        environment["TESTDESK_CONFIG_HOME"] = str(config_dir)
        first = fixture(Path(directory), 'first')
        second = fixture(Path(directory), 'second')
        with tempfile.TemporaryFile(mode="w+b") as log:
            process = subprocess.Popen(
                [str(executable), "--agent-start", "--project", first],
                stdin=subprocess.DEVNULL,
                stdout=log,
                stderr=subprocess.STDOUT,
                env=environment,
            )
            try:
                deadline = time.monotonic() + 45
                session = None
                while time.monotonic() < deadline:
                    if process.poll() is not None:
                        raise RuntimeError(f"desktop exited before MCP readiness (exit {process.returncode})")
                    session = open_mcp(config_dir / "TestDesk" / "desktop-lifecycle.json", process.pid)
                    if session is not None:
                        break
                    time.sleep(0.2)
                if session is None:
                    raise RuntimeError("desktop did not expose a tray-ready MCP session within 45 seconds")
                client = Client(*session)
                try:
                    exercise(client, first, second)
                finally:
                    client.close()
                session = open_mcp(config_dir / "TestDesk" / "desktop-lifecycle.json", process.pid)
                if session is None:
                    raise RuntimeError('desktop did not accept a reconnect')
                client = Client(*session)
                try:
                    client.tool('testdesk_snapshot', project=second)
                finally:
                    client.close()
                print("Packaged desktop passed tray readiness, discovery, exact pass/fail, cancellation, multi-project isolation and MCP reconnect checks.")
            except Exception:
                log.seek(0)
                output = log.read(8192).decode(errors="replace")
                if output:
                    print("Desktop output (first 8192 bytes):\n" + output, file=sys.stderr)
                raise
            finally:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)


if __name__ == "__main__":
    main()

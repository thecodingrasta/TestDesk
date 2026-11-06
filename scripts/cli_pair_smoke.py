#!/usr/bin/env python3
"""Check that a standalone CLI beside a standalone desktop starts it and serves MCP.

This is what a user gets after putting the two release downloads in one PATH directory.
"""
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import threading
import time


def main():
    if len(sys.argv) != 2:
        raise SystemExit('usage: cli_pair_smoke.py <directory holding testdesk-cli and testdesk>')
    folder = Path(sys.argv[1]).resolve(strict=True)
    suffix = '.exe' if sys.platform == 'win32' else ''
    cli = folder / f'testdesk-cli{suffix}'
    with tempfile.TemporaryDirectory(prefix='testdesk-pair-', ignore_cleanup_errors=True) as scratch:
        config = Path(scratch) / 'config'
        environment = dict(os.environ, TESTDESK_CONFIG_HOME=str(config))
        environment.pop('TESTDESK_DESKTOP_EXECUTABLE', None)
        process = subprocess.Popen([str(cli), 'mcp'], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, env=environment, cwd=folder)
        watchdog = threading.Timer(90, process.kill)
        watchdog.start()
        try:
            def call(identifier, method, params=None):
                message = {'jsonrpc': '2.0', 'id': identifier, 'method': method, 'params': params or {}}
                process.stdin.write((json.dumps(message) + '\n').encode())
                process.stdin.flush()
                line = process.stdout.readline()
                if not line:
                    raise RuntimeError('the CLI closed the session: ' + process.stderr.read().decode(errors='replace')[:500])
                return json.loads(line)

            started = time.monotonic()
            call(1, 'initialize', {'protocolVersion': '2025-06-18', 'capabilities': {},
                                  'clientInfo': {'name': 'pair-check', 'version': '1'}})
            process.stdin.write(b'{"jsonrpc":"2.0","method":"notifications/initialized"}\n')
            process.stdin.flush()
            names = [tool['name'] for tool in call(2, 'tools/list')['result']['tools']]
            if 'testdesk_run' not in names:
                raise RuntimeError(f'unexpected MCP tool list: {names}')
            print(f'The CLI started the desktop beside it and served {len(names)} tools in {time.monotonic() - started:.1f}s.')
        finally:
            watchdog.cancel()
            process.kill()
            process.wait(timeout=10)
            # The CLI leaves the tray desktop running by design; stop the one this check started.
            lease = config / 'TestDesk' / 'desktop-lifecycle.json'
            try:
                pid = json.loads(lease.read_text(encoding='utf-8'))['pid']
                os.kill(pid, signal.SIGTERM)
                time.sleep(1)
            except (FileNotFoundError, KeyError, ValueError, OSError):
                pass


if __name__ == '__main__':
    main()

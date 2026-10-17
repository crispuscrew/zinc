"""Keep the shared fixture with its transport cases instead of a separate support module."""
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

TOOLS = ('zc', 'zcr', 'zlg', 'zlt', 'zvr')
INSTALLER = Path(__file__).with_name('zinc-net-install.sh').resolve()
FIXTURE = '''#!/usr/bin/env python3
import os, pathlib, shutil, sys
root = pathlib.Path(os.environ['FIXTURE_ROOT'])
if pathlib.Path(sys.argv[0]).name == 'uname':
    print({'-s': os.environ.get('SYSTEM', 'Linux'), '-m': os.environ.get('MACHINE', 'x86_64')}[sys.argv[1]])
    sys.exit(int(os.environ.get('UNAME_STATUS', '0')))
args = sys.argv[1:]
asset = args[-1].rsplit('/', 1)[-1]
assert args[:-2] == '-q --fail --silent --show-error --location --proto =https --proto-redir =https --output'.split()
if asset == 'zinc-hello.yaml':
    assert args[-1] == 'https://raw.githubusercontent.com/crispuscrew/zinc/' + os.environ['VERSION'] + '/common/examples/apps/zinc-hello.yaml'
else:
    assert asset in ['zc-linux-amd64', 'zcr-linux-amd64', 'zlg-linux-amd64', 'zlt-linux-amd64', 'zvr-linux-amd64', 'SHA256SUMS']
    assert args[-1] == 'https://github.com/crispuscrew/zinc/releases/download/' + os.environ['VERSION'] + '/' + asset
with (root / 'downloads').open('a') as stream: stream.write(asset + '\\n')
if asset == os.environ.get('NETWORK'): sys.exit(22)
shutil.copyfile(root / 'assets' / asset, args[-2])
'''

class InstallerTests(unittest.TestCase):
    def prepare(self, version='v0.11.1', bin_name=None, existing=False):
        scratch_root = Path(os.environ.get('TMPDIR') or '/tmp/opencode')
        scratch_root.mkdir(parents=True, exist_ok=True)
        scratch = tempfile.TemporaryDirectory(prefix='zinc-test-', dir=scratch_root)
        self.addCleanup(scratch.cleanup)
        self.root = Path(scratch.name)
        self.home, self.temp, self.assets, fake = (self.root / name for name in ('home', 'tmp', 'assets', 'fake'))
        for directory in (self.home, self.temp, self.assets, fake): directory.mkdir()
        self.bin = self.root / bin_name if bin_name else self.home / '.local/bin'
        self.env = dict(os.environ, HOME=str(self.home), TMPDIR=str(self.temp), FIXTURE_ROOT=str(self.root),
                        VERSION=version, PATH=str(fake) + ':/usr/bin:/bin', SYSTEM='Linux', MACHINE='x86_64',
                        UNAME_STATUS='0', NETWORK='')
        self.env.update(SHELL='/bin/bash', ZDOTDIR=str(self.home), XDG_CONFIG_HOME=str(self.home / '.config'))
        for name in ('curl', 'uname'):
            (fake / name).write_text(FIXTURE)
            (fake / name).chmod(0o755)
        for tool in TOOLS:
            (self.assets / (tool + '-linux-amd64')).write_text(
                '#!/bin/sh\n[ "$#" -eq 1 ] && [ "$1" = version ] || exit 64\n'
                f'printf "%s\\n" {tool} >> "$FIXTURE_ROOT/runs"\nprintf "%s\\n" "{tool} {version}"\n')
        self.manifest()
        if existing:
            self.bin.mkdir(parents=True)
            for tool in TOOLS:
                (self.bin / tool).write_text('old ' + tool)
                (self.bin / tool).chmod(0o750)
        return self.snapshot() if existing else {}

    def manifest(self):
        entries = [f'{hashlib.sha256(asset.read_bytes()).hexdigest()}  {asset.name}\n' for asset in sorted(self.assets.glob('*-linux-amd64'))]
        (self.assets / 'SHA256SUMS').write_text(''.join(entries))

    def snapshot(self, tools=TOOLS):
        return {tool: ((self.bin / tool).read_bytes(), (self.bin / tool).stat().st_mode,
                       (self.bin / tool).stat().st_ino) for tool in tools}

    def invoke(self, *args, conditional=False, example=False, response='n\n'):
        options = [] if example else ['--no-example']
        command = ['sh', str(INSTALLER), *options, *args]
        if conditional:
            command = ['sh', '-c', 'installer=$1; shift; (set -e; . "$installer") && exit 0; exit 1',
                       'sh', str(INSTALLER), *options, *args]
        result = subprocess.run(command, cwd=self.root, env=self.env, input=response, capture_output=True, text=True, timeout=20)
        self.assertEqual((list(self.temp.iterdir()), list(self.bin.glob('.zinc-install.*'))), ([], []))
        return result

    def test_install_and_reinstall(self):
        for bin_name, version, mode in ((None, 'v0.11.1', 0o755), ('custom bin', 'v1.2.3', 0o750)):
            with self.subTest(bin_name=bin_name, version=version):
                before = self.prepare(version, bin_name, existing=bool(bin_name))
                args = ['--version', version[1:], '--bin-dir', bin_name] if bin_name else []
                held = self.root / 'held-zc'
                if bin_name:
                    os.link(self.bin / 'zc', held)
                    self.env.pop('HOME')
                    self.env['MACHINE'] = 'amd64'
                for attempt in range(2):
                    result = self.invoke(*args)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual({entry.name for entry in self.bin.iterdir()}, set(TOOLS))
                    self.assertEqual((self.root / 'downloads').read_text().splitlines(),
                                     ([tool + '-linux-amd64' for tool in TOOLS] + ['SHA256SUMS']) * (attempt + 1))
                    self.assertEqual((self.root / 'runs').read_text().splitlines(), list(TOOLS) * (2 * attempt + 1))
                    for tool in TOOLS:
                        self.assertEqual(subprocess.check_output([str(self.bin / tool), 'version'], env=self.env), f'{tool} {version}\n'.encode())
                        self.assertEqual((self.bin / tool).stat().st_mode & 0o777, mode)
                if bin_name: self.assertEqual(held.read_bytes(), before['zc'][0])

    def test_verification_failures_preserve_tools(self):
        cases = ('network', 'missing', 'duplicate', 'hash path', 'bad hash', 'checksum', 'version', 'version exit', 'blank')
        for failure in cases:
            with self.subTest(failure=failure):
                before = self.prepare(existing=True)
                asset, manifest = self.assets / 'zvr-linux-amd64', self.assets / 'SHA256SUMS'
                lines = manifest.read_text().splitlines(keepends=True)
                if failure == 'network': self.env['NETWORK'] = asset.name
                elif failure == 'missing': manifest.write_text(''.join(lines[:-1]))
                elif failure == 'duplicate': manifest.write_text(''.join(lines[:-1] + lines[:1]))
                elif failure == 'hash path': manifest.write_text(''.join(lines[:-1]) + '0' * 64 + '  ../home/.local/bin/zc\n')
                elif failure == 'bad hash': manifest.write_text(''.join(lines[:-1]) + 'g' * 64 + '  zvr-linux-amd64\n')
                else:
                    asset.write_text(asset.read_text().replace('v0.11.1', 'v9.9.9') if failure == 'version'
                                     else asset.read_text() + ('exit 1\n' if failure == 'version exit' else "printf '\\n'\n"))
                    if failure != 'checksum': self.manifest()
                result = self.invoke(conditional=failure in ('network', 'checksum', 'version exit'))
                self.assertNotEqual(result.returncode, 0, result.stdout)
                if failure in ('missing', 'duplicate', 'hash path', 'bad hash'):
                    self.assertIn('invalid SHA256SUMS manifest', result.stderr)
                self.assertEqual(self.snapshot(), before)
                self.assertEqual((self.root / 'runs').read_text().splitlines() if (self.root / 'runs').exists() else [],
                                 list(TOOLS) if failure in ('version', 'version exit', 'blank') else [])

    def test_invalid_input(self):
        cases = [(('--example',), {}), (('--version',), {}), (('--bin-dir', ''), {}), (('--bin-dir', '--help'), {}),
                 (('--version', '01.2.3'), {}), (('--version', '1.2.3-rc.1'), {}), (('--version', '1.2.3\n4.5.6'), {}),
                 ((), {'SYSTEM': 'Darwin'}), ((), {'MACHINE': 'aarch64'}), ((), {'UNAME_STATUS': '1'}), ((), {'HOME': 'relative'})]
        for args, environment in cases:
            with self.subTest(args=args, environment=environment):
                before = self.prepare(existing=True)
                self.env.update(environment)
                self.assertNotEqual(self.invoke(*args).returncode, 0)
                self.assertEqual(self.snapshot(), before)
                self.assertFalse((self.root / 'downloads').exists())

    def test_reject_special_destinations(self):
        for kind in ('symlink', 'directory', 'fifo', 'nonexecutable', 'bin symlink'):
            with self.subTest(kind=kind):
                self.prepare(existing=True)
                before = self.snapshot(TOOLS[:-1])
                target = self.bin / 'zvr'
                target.unlink()
                if kind == 'symlink': target.symlink_to(self.root / 'missing')
                elif kind == 'directory': target.mkdir()
                elif kind == 'fifo': os.mkfifo(target)
                else: target.write_text('old zvr'); target.chmod(0o750 if kind == 'bin symlink' else 0o644)
                if kind == 'bin symlink':
                    self.bin.rename(self.root / 'real-bin')
                    self.bin.symlink_to(self.root / 'real-bin')
                metadata = target.lstat()
                self.assertNotEqual(self.invoke().returncode, 0)
                self.assertEqual(self.snapshot(TOOLS[:-1]), before)
                self.assertEqual(target.lstat(), metadata)
                self.assertFalse((self.root / 'downloads').exists())

if __name__ == '__main__':
    unittest.main(verbosity=2)

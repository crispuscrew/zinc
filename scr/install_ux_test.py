import os
from pathlib import Path
import subprocess
import unittest

import install_test

PIN = 'docker.io/library/alpine@sha256:' + 'a' * 64
CLIENT = '''#!/usr/bin/env python3
import os, pathlib, sys
root = pathlib.Path(os.environ['FIXTURE_ROOT'])
args = sys.argv[1:]
if args == ['version']:
    print('zc v0.11.1')
elif args == ['image', 'resolve', 'docker.io/library/alpine:3.22']:
    if os.environ.get('RESOLVE_FAILURE'): sys.exit(1)
    print('docker.io/library/alpine@sha256:' + 'a' * 64)
elif args[0] == 'validate':
    assert 'AppNameID: zinc-hello' in pathlib.Path(args[1]).read_text()
    print(pathlib.Path(args[1]).read_text())
elif args[0] == 'run':
    (root / 'launched').write_text(' '.join(args))
else:
    sys.exit(64)
'''


class InstallerUXTests(unittest.TestCase):
    def fixture(self, directory=None):
        fixture = install_test.InstallerTests()
        self.addCleanup(fixture.doCleanups)
        fixture.prepare(bin_name=directory)
        fixture.env.update(SHELL='/bin/bash', ZINC_TERMINAL='test-terminal', DISPLAY=':test')
        (fixture.assets / 'zc-linux-amd64').write_text(CLIENT)
        podman = fixture.root / 'fake/podman'
        podman.write_text('#!/bin/sh\n[ "$1 $2" = "image exists" ]\n')
        podman.chmod(0o755)
        (fixture.assets / 'zinc-hello.yaml').write_text(
            'SchemaVersion: 4\nType: ZincContainer\nAppNameID: zinc-hello\n'
            'ImageMeta:\n  Image: docker.io/library/alpine@sha256:' + '0' * 64 + '\n'
            'StartConditions:\n  Entrypoint: /bin/sh\n  Terminal: true\n'
            'DisplayMeta:\n  DisableGpuAccess: true\n')
        fixture.manifest()
        return fixture

    def test_path_is_persistent_quoted_and_idempotent(self):
        fixture = self.fixture("bin ' quoted $ value")
        profile = fixture.home / '.bashrc'
        profile.write_text('# user settings without final newline')
        args = ['--bin-dir', str(fixture.bin)]
        for attempt in range(2):
            result = fixture.invoke(*args)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(profile.read_text().count('export PATH='), 1)
            self.assertTrue(profile.read_text().startswith('# user settings without final newline\n'))
        found = subprocess.check_output(
            ['bash', '--noprofile', '--norc', '-c', '. "$HOME/.bashrc"; command -v zc'],
            env=fixture.env, text=True)
        self.assertEqual(found.strip(), str(fixture.bin / 'zc'))

    def test_profile_backslash_without_newline_is_preserved(self):
        for ending in ('\\', '\\\\', '\\\\\\', '\\\n'):
            with self.subTest(ending=ending):
                fixture = self.fixture()
                profile = fixture.home / '.bashrc'
                original = 'export PROFILE_MARK=kept' + ending
                profile.write_text(original)
                result = fixture.invoke()
                unsafe = ending in ('\\', '\\\\\\')
                self.assertEqual(result.returncode != 0, unsafe, result.stderr)
                if unsafe:
                    self.assertEqual(profile.read_text(), original)
                else:
                    self.assertTrue(profile.read_text().startswith(original))

    def test_profile_search_error_does_not_append(self):
        fixture = self.fixture()
        profile = fixture.home / '.bashrc'
        profile.write_text('# unchanged\n')
        search = fixture.root / 'fake/grep'
        search.write_text('#!/bin/sh\nexit 2\n')
        search.chmod(0o755)
        for attempt in range(2):
            result = fixture.invoke()
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('cannot search shell startup file', result.stderr)
            self.assertEqual(profile.read_text(), '# unchanged\n')

    @unittest.skipIf(os.geteuid() == 0, 'root can read write-only profiles')
    def test_write_only_profile_is_preserved(self):
        fixture = self.fixture()
        profile = fixture.home / '.bashrc'
        profile.write_text('# unchanged\n')
        profile.chmod(0o200)
        result = fixture.invoke()
        profile.chmod(0o600)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('cannot read shell startup file', result.stderr)
        self.assertEqual(profile.read_text(), '# unchanged\n')

    def test_zsh_and_fish_startup_locations(self):
        for shell, path in (('zsh', '.zshrc'), ('fish', '.config/fish/conf.d/zinc.fish')):
            with self.subTest(shell=shell):
                fixture = self.fixture()
                fixture.env['SHELL'] = '/bin/' + shell
                result = fixture.invoke()
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(str(fixture.bin), (fixture.home / path).read_text())

    def test_unsupported_shell_does_not_modify_startup_files(self):
        fixture = self.fixture()
        fixture.env['SHELL'] = '/bin/unknown-shell'
        result = fixture.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('unsupported for shell', result.stderr)
        self.assertFalse((fixture.home / '.profile').exists())
        self.assertFalse((fixture.root / 'launched').exists())

    def test_preview_and_decline_do_not_launch(self):
        fixture = self.fixture()
        result = fixture.invoke(example=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        sample = fixture.root / 'zinc-hello.yaml'
        self.assertIn(PIN, sample.read_text())
        self.assertIn(sample.read_text(), result.stdout)
        self.assertIn('Run this example? [y/N]', result.stdout)
        self.assertFalse((fixture.root / 'launched').exists())
        self.assertFalse(list(fixture.root.glob('.zinc-hello.*')))

    def test_only_explicit_yes_launches(self):
        for response in ('y\n', 'yes\n', '\n', '', 'no\n'):
            with self.subTest(response=response):
                fixture = self.fixture()
                result = fixture.invoke(example=True, response=response)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((fixture.root / 'launched').exists(), response in ('y\n', 'yes\n'))

    def test_existing_yaml_is_preserved(self):
        fixture = self.fixture()
        sample = fixture.root / 'zinc-hello.yaml'
        original = (fixture.assets / 'zinc-hello.yaml').read_bytes()
        sample.write_bytes(original)
        fixture.env['RESOLVE_FAILURE'] = '1'
        result = fixture.invoke(example=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(sample.read_bytes(), original)
        self.assertNotIn('zinc-hello.yaml', (fixture.root / 'downloads').read_text())

    def test_sample_failures_never_launch(self):
        for failure in ('download', 'resolve'):
            with self.subTest(failure=failure):
                fixture = self.fixture()
                fixture.env['NETWORK' if failure == 'download' else 'RESOLVE_FAILURE'] = (
                    'zinc-hello.yaml' if failure == 'download' else '1')
                result = fixture.invoke(example=True, response='yes\n')
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((fixture.root / 'zinc-hello.yaml').exists())
                self.assertFalse((fixture.root / 'launched').exists())


if __name__ == '__main__':
    unittest.main(verbosity=2)

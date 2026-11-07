#!/usr/bin/env python3
"""Verify release layout/checksums and smoke-test only this host's executable."""
import argparse
import hashlib
import json
import pathlib
import platform
import subprocess
import tarfile
import tempfile
import zipfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify(directory, version, native):
    expected = {}
    for os_name in ('linux', 'darwin', 'windows'):
        for arch in ('amd64', 'arm64'):
            suffix = '.zip' if os_name == 'windows' else '.tar.gz'
            expected[f'graybox_{version.removeprefix("v")}_{os_name}_{arch}{suffix}'] = (os_name, arch)
    manifest = {}
    for line in (directory / 'SHA256SUMS').read_text().splitlines():
        checksum, name = line.split('  ')
        if name in manifest or name not in expected:
            raise ValueError('unexpected or duplicate checksum entry')
        manifest[name] = checksum
    system = platform.system().lower()
    machine = {'x86_64': 'amd64', 'amd64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}[platform.machine().lower()]
    required = {name for name, target in expected.items() if not native or target == (system, machine)}
    if set(manifest) != required or {p.name for p in directory.iterdir()} != required | {'SHA256SUMS'}:
        raise ValueError('archive set does not match expected targets')
    executed = False
    for name in sorted(required):
        path = directory / name
        if digest(path) != manifest[name]:
            raise ValueError(f'checksum mismatch: {name}')
        os_name, arch = expected[name]
        executable = 'graybox.exe' if os_name == 'windows' else 'graybox'
        if path.suffix == '.zip':
            with zipfile.ZipFile(path) as archive:
                if archive.namelist() != [executable]:
                    raise ValueError(f'unexpected archive layout: {name}')
                data = archive.read(executable)
        else:
            with tarfile.open(path, 'r:gz') as archive:
                members = archive.getmembers()
                if len(members) != 1 or members[0].name != executable or not members[0].isfile() or not members[0].mode & 0o111:
                    raise ValueError(f'unexpected archive layout/mode: {name}')
                data = archive.extractfile(members[0]).read()
        if not data:
            raise ValueError(f'empty executable: {name}')
        if (os_name, arch) == (system, machine):
            with tempfile.TemporaryDirectory() as temp:
                binary = pathlib.Path(temp) / executable
                binary.write_bytes(data)
                binary.chmod(0o700)
                def run(*args):
                    return subprocess.run([str(binary), *args], check=True, capture_output=True, text=True, timeout=15).stdout
                if run('version').strip() != f'graybox {version}':
                    raise ValueError('wrong embedded version')
                if json.loads(run('version', '--json')) != {'version': version}:
                    raise ValueError('wrong JSON version')
                for args in [[], ['--help'], *[['help', c] for c in ('record', 'ls', 'show', 'replay', 'diff', 'version')]]:
                    if 'Usage:' not in run(*args):
                        raise ValueError(f'missing help: {args}')
            executed = True
        print(f'OK {name}')
    if not executed:
        raise ValueError('no native executable tested')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=pathlib.Path)
    parser.add_argument('version', nargs='?')
    parser.add_argument('--native-only', action='store_true')
    parser.add_argument('--write-checksums', action='store_true')
    args = parser.parse_args()
    if args.write_checksums:
        archives = sorted(p for p in args.directory.iterdir() if p.name.endswith(('.tar.gz', '.zip')))
        (args.directory / 'SHA256SUMS').write_text(''.join(f'{digest(p)}  {p.name}\n' for p in archives))
    else:
        if not args.version:
            parser.error('version is required for verification')
        verify(args.directory, args.version, args.native_only)

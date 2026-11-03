#!/usr/bin/env python3
"""Embed THIS SOURCE literally in the approved python3 -I -c command; never run staging code first."""
import hashlib
import os
from pathlib import Path
import re
import stat
import sys
import types


def require_root_path(path, directory=False):
    s = path.lstat()
    kind = stat.S_ISDIR if directory else stat.S_ISREG
    if not kind(s.st_mode) or s.st_uid != 0 or s.st_mode & 0o022:
        raise RuntimeError(f'not a protected root-owned path: {path}')


def verified_bundle(directory, digest):
    path = Path(directory)
    if not path.is_absolute() or '..' in path.parts or not re.fullmatch('[0-9a-f]{64}', digest):
        raise ValueError('absolute canonical directory and pinned SHA256 required')
    for ancestor in reversed([path, *path.parents]):
        require_root_path(ancestor, directory=True)
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    def read(name):
        f = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        with os.fdopen(f, 'rb') as stream:
            s = os.fstat(stream.fileno())
            if not stat.S_ISREG(s.st_mode) or s.st_uid != 0 or s.st_mode & 0o022 or s.st_size > 1048576:
                raise RuntimeError(f'unsafe bundle member: {name}')
            return stream.read(1048577)
    try:
        manifest = read('SHA256SUMS')
        if hashlib.sha256(manifest).hexdigest() != digest:
            raise RuntimeError('manifest differs from approved digest')
        bundle = {}
        for line in manifest.decode('ascii').splitlines():
            match = re.fullmatch(r'([0-9a-f]{64})  ([A-Za-z0-9_-][A-Za-z0-9_.-]*)', line)
            if not match or match[2] in bundle or match[2] == 'SHA256SUMS':
                raise ValueError('invalid or duplicate manifest entry')
            data = read(match[2])
            if hashlib.sha256(data).hexdigest() != match[1]:
                raise RuntimeError(f'payload digest mismatch: {match[2]}')
            bundle[match[2]] = data
        return bundle
    finally:
        os.close(fd)


def main():
    if os.geteuid() != 0:
        raise RuntimeError('root required')
    digest, directory, action, backup = sys.argv[1:]
    bundle = verified_bundle(directory, digest)
    # Imports and payload data are the same verified bytes, never reread from staging.
    for name in ('guard_files', 'guard_checks'):
        module = types.ModuleType(name)
        exec(compile(bundle[name + '.py'], name + '.py', 'exec'), module.__dict__)
        sys.modules[name] = module
    scope = {'__name__': '__main__', 'BUNDLE': bundle, 'BUNDLE_DIGEST': digest}
    sys.argv = ['guard_change.py', action, backup]
    exec(compile(bundle['guard_change.py'], 'guard_change.py', 'exec'), scope)


if __name__ == '__main__':
    main()

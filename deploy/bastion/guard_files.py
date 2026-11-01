#!/usr/bin/env python3
"""Root-only atomic file operations. Parents must already exist; no mkdir side effects."""
import hashlib
import json
import os
from pathlib import Path
import stat
import sys


def protected(path, directory=False):
    p = Path(path)
    for parent in reversed(p.parents):
        s = parent.lstat()
        if not stat.S_ISDIR(s.st_mode) or s.st_uid != 0 or s.st_mode & 0o022:
            raise RuntimeError(f"unsafe parent: {parent}")
    s = p.lstat()
    kind = stat.S_ISDIR if directory else stat.S_ISREG
    if not kind(s.st_mode) or s.st_uid != 0 or s.st_mode & 0o022:
        raise RuntimeError(f"unsafe path: {p}")


def sync_dir(path):
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def pending(path):
    return path.with_name('.' + path.name + '.shippin-pending')


def install(path, content):
    """Final name is either absent or complete, including on short writes/process death."""
    path = Path(path)
    protected(path.parent, directory=True)
    temp = pending(path)
    fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        data = memoryview(content.encode())
        while data:
            count = os.write(fd, data)
            if count <= 0:
                raise OSError('write made no progress')
            data = data[count:]
        os.fchmod(fd, 0o644)
        os.fsync(fd)
    finally:
        os.close(fd)
    os.link(temp, path, follow_symlinks=False)  # atomic, no replacement of existing target
    sync_dir(path.parent)
    temp.unlink()
    sync_dir(path.parent)


def validate(files, absent=False):
    for name, content in files.items():
        path = Path(name)
        protected(path.parent, directory=True)
        for candidate in (path, pending(path)):
            if os.path.lexists(candidate):
                protected(candidate)
                if absent:
                    raise RuntimeError(f'path already exists: {candidate}')
        if not absent and path.exists():
            if hashlib.sha256(path.read_bytes()).digest() != hashlib.sha256(content.encode()).digest():
                raise RuntimeError(f'file drift; refusing removal: {path}')


def remove(files):
    validate(files)  # all paths checked before any removal
    for name in files:
        path = Path(name)
        path.unlink(missing_ok=True)
        pending(path).unlink(missing_ok=True)  # planned, exclusive, root-only partial write
        sync_dir(path.parent)


def atomic_json(path, value):
    """Durably replace the private journal; an interrupted update retains the old journal."""
    path = Path(path)
    protected(path.parent, directory=True)
    temp = pending(path)
    if os.path.lexists(temp):
        protected(temp)
        temp.unlink()
    fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as f:
        json.dump(value, f)
        f.flush()
        os.fsync(f.fileno())
    os.replace(temp, path)
    sync_dir(path.parent)


def main():
    if os.geteuid() != 0:
        raise RuntimeError('root required')
    action, data = sys.argv[1], json.loads(sys.argv[2])
    if action == 'check':
        validate(data, absent=True)
    elif action == 'validate':
        validate(data)
    elif action == 'install':
        for name, content in data.items():
            install(name, content)
    elif action == 'remove':
        remove(data)
    else:
        raise ValueError('unknown file operation')


if __name__ == '__main__':
    main()

"""Linux root-only fault injection in a private temporary directory, never /etc."""
import hashlib
import multiprocessing
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import guard_files as files
from guard_launcher import verified_bundle


class AtomicFileTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='guard-test-', dir='/root')
        self.root = Path(self.temp.name)
        self.target = self.root / 'unit.service'
        self.content = 'complete configuration\n' * 100

    def tearDown(self):
        self.temp.cleanup()

    def test_short_writes_are_completed(self):
        original = os.write
        with patch.object(os, 'write', lambda fd, data: original(fd, data[:7])):
            files.install(self.target, self.content)
        self.assertEqual(self.target.read_text(), self.content)

    def test_disk_full_leaves_no_partial_final_and_can_rollback(self):
        original, calls = os.write, []
        def fail(fd, data):
            calls.append(1)
            if len(calls) > 1:
                raise OSError(28, 'injected ENOSPC')
            return original(fd, data[:7])
        with patch.object(os, 'write', fail), self.assertRaises(OSError):
            files.install(self.target, self.content)
        self.assertFalse(self.target.exists())
        files.remove({str(self.target): self.content})
        self.assertFalse(files.pending(self.target).exists())

    def test_process_death_after_every_file_mutation_rolls_back(self):
        # True child process death after write/chmod/fsync/link/unlink, not exceptions/finally.
        for name, occurrence in [('write', 1), ('fchmod', 1), ('fsync', 1),
                                 ('link', 1), ('fsync', 2), ('unlink', 1), ('fsync', 3)]:
            with self.subTest(name=name, occurrence=occurrence):
                def child():
                    original, count = getattr(os, name), 0
                    def kill(*args, **kwargs):
                        nonlocal count
                        result = original(*args, **kwargs)
                        count += 1
                        if count == occurrence:
                            os._exit(77)
                        return result
                    with patch.object(os, name, kill):
                        files.install(self.target, self.content)
                p = multiprocessing.Process(target=child)
                p.start(); p.join(5)
                self.assertEqual(p.exitcode, 77)
                if self.target.exists():
                    self.assertEqual(self.target.read_text(), self.content)
                files.remove({str(self.target): self.content})
                self.assertEqual(list(self.root.iterdir()), [])

    def test_existing_drift_and_symlinks_are_never_overwritten_or_removed(self):
        self.target.write_text('unrelated')
        with self.assertRaises(FileExistsError):
            files.install(self.target, self.content)
        with self.assertRaises(RuntimeError):
            files.remove({str(self.target): self.content})
        self.assertEqual(self.target.read_text(), 'unrelated')
        self.target.unlink()
        self.target.symlink_to('/dev/null')
        with self.assertRaises(RuntimeError):
            files.remove({str(self.target): self.content})

    def test_journal_interruption_preserves_valid_old_journal(self):
        marker = self.root / 'prepared.json'
        files.atomic_json(marker, {'state': 'prepared'})
        with patch.object(os, 'replace', side_effect=OSError('interrupted')), self.assertRaises(OSError):
            files.atomic_json(marker, {'state': 'applied'})
        self.assertEqual(marker.read_text(), '{"state": "prepared"}')
        files.atomic_json(marker, {'state': 'rolled_back'})
        self.assertEqual(marker.read_text(), '{"state": "rolled_back"}')

    def test_bundle_rejects_tamper_symlink_writable_and_wrong_pin(self):
        self.target.write_text('reviewed bytes')
        manifest = f'{hashlib.sha256(self.target.read_bytes()).hexdigest()}  unit.service\n'.encode()
        (self.root / 'SHA256SUMS').write_bytes(manifest)
        digest = hashlib.sha256(manifest).hexdigest()
        self.assertEqual(verified_bundle(self.root, digest)['unit.service'], b'reviewed bytes')
        with self.assertRaises(RuntimeError):
            verified_bundle(self.root, '0' * 64)
        self.target.chmod(0o666)
        with self.assertRaises(RuntimeError):
            verified_bundle(self.root, digest)
        self.target.chmod(0o644)
        self.target.write_text('replaced bytes')
        with self.assertRaises(RuntimeError):
            verified_bundle(self.root, digest)
        self.target.unlink(); self.target.symlink_to('/dev/null')
        with self.assertRaises(OSError):
            verified_bundle(self.root, digest)


if __name__ == '__main__':
    unittest.main()

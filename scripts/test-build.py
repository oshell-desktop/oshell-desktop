"""Check portable publication without touching real application data."""
import pathlib
import io
import hashlib
import json
import tempfile
import unittest
from contextlib import nullcontext
from unittest.mock import patch

import build
from single_file import embedded_resources


class PortablePublishTests(unittest.TestCase):
    def test_unsupported_targets_fail_before_building(self):
        with patch.object(build, 'run') as run:
            for target, single in [('darwin/arm64', False), ('linux/amd64', True)]:
                with self.assertRaises(SystemExit):
                    build.build(target, pathlib.Path('unused'), single_file=single)
            run.assert_not_called()

    def test_linux_package_preserves_runtime_and_execute_permission(self):
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            (root / 'mygo.ts').write_text('original client')

            def fake_build(*args, **kwargs):
                stage = pathlib.Path(args[args.index('-o') + 1]) / 'linux-amd64'
                stage.mkdir(parents=True)
                (root / 'mygo.ts').write_text('generated')
                (stage / 'oshell-desktop.deb').write_bytes(b'installer')
                with build.tarfile.open(stage / 'oshell-desktop-0.3.0-linux-amd64.tar.gz', 'w:gz') as out:
                    for name, data in [('oshell-desktop', b'ELF-program'), ('libghostty-vt.so', b'ELF-library')]:
                        member = build.tarfile.TarInfo(name)
                        member.size, member.mode = len(data), 0o644
                        out.addfile(member, io.BytesIO(data))

            with patch.object(build, 'ROOT', root), patch.object(build, 'run', fake_build), patch.object(build, 'native_manifest', return_value=nullcontext()):
                build.build('linux/amd64', root / 'dist', archive=True)
            self.assertEqual((root / 'mygo.ts').read_text(), 'original client')
            self.assertFalse((root / 'dist/linux-amd64/oshell-portable').exists())
            with build.tarfile.open(root / 'dist/OShell-Desktop-linux-amd64.tar.gz', 'r:gz') as archive:
                self.assertEqual(set(archive.getnames()), {'oshell-desktop', 'libghostty-vt.so'})
                self.assertEqual(archive.getmember('oshell-desktop').mode, 0o755)
                self.assertEqual(archive.extractfile('libghostty-vt.so').read(), b'ELF-library')

    def fixture(self, root):
        source, destination = root / 'incoming', root / 'published'
        source.mkdir()
        (source / 'oshell-portable').write_text('portable')
        (source / 'OShell Desktop.exe').write_bytes(b'new-program')
        destination.mkdir()
        (destination / 'oshell-portable').write_text('portable')
        (destination / 'OShell Desktop.exe').write_bytes(b'old-program')
        return source, destination

    def test_rebuild_does_not_erase_portable_user_data(self):
        with tempfile.TemporaryDirectory() as folder:
            source, destination = self.fixture(pathlib.Path(folder))
            (destination / 'config.json').write_bytes(b'user-configuration')
            with self.assertRaisesRegex(RuntimeError, '--out'):
                build.publish(source, destination)
            self.assertEqual((destination / 'config.json').read_bytes(), b'user-configuration')
            self.assertEqual((destination / 'OShell Desktop.exe').read_bytes(), b'old-program')
            self.assertFalse((source / 'config.json').exists())

    def test_clean_build_can_be_replaced(self):
        with tempfile.TemporaryDirectory() as folder:
            source, destination = self.fixture(pathlib.Path(folder))
            build.publish(source, destination)
            self.assertEqual((destination / 'OShell Desktop.exe').read_bytes(), b'new-program')

    def test_single_file_rebuild_preserves_user_data_without_marker(self):
        with tempfile.TemporaryDirectory() as folder:
            source, destination = self.fixture(pathlib.Path(folder))
            (source / 'oshell-portable').unlink()
            (destination / 'oshell-portable').unlink()
            (destination / 'credentials.json').write_bytes(b'private')
            with self.assertRaisesRegex(RuntimeError, '--out'):
                build.publish(source, destination)
            self.assertEqual((destination / 'credentials.json').read_bytes(), b'private')

    def test_embed_resources_verified_and_removed_after_failure(self):
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            package = root / 'internal/bundled'
            package.mkdir(parents=True)
            manifest = root / 'internal/terminal/mygo-plugin.json'
            manifest.parent.mkdir()
            manifest.write_text(json.dumps({'libraries': [{'name': 'libghostty-vt', 'files': {'windows-amd64': {'name': 'ghostty-vt.dll', 'sha256': hashlib.sha256(b'native').hexdigest()}}}]}))
            product = root / 'product'
            product.mkdir()
            (product / 'ghostty-vt.dll').write_bytes(b'native')
            (product / 'OShell Desktop.exe').write_bytes(b'program')
            with self.assertRaisesRegex(RuntimeError, 'compile failure'):
                with embedded_resources(root, product):
                    with build.zipfile.ZipFile(package / 'payload_windows_amd64.zip') as z:
                        self.assertEqual(z.namelist(), ['ghostty-vt.dll'])
                    raise RuntimeError('compile failure')
            self.assertEqual(list(package.iterdir()), [])
            (product / 'config.json').write_text('private')
            with self.assertRaisesRegex(RuntimeError, 'User data'):
                with embedded_resources(root, product):
                    self.fail('embedded private data')
            (product / 'config.json').unlink()
            (product / 'ghostty-vt.dll').write_bytes(b'corrupt')
            with self.assertRaisesRegex(RuntimeError, 'checksum'):
                with embedded_resources(root, product):
                    self.fail('embedded wrong DLL')

    def test_failed_publication_restores_previous_build(self):
        with tempfile.TemporaryDirectory() as folder:
            source, destination = self.fixture(pathlib.Path(folder))
            rename = pathlib.Path.rename

            def fail_incoming(path, target):
                if path.name == destination.name and path.parent.name.startswith('.oshell-publish-'):
                    raise OSError('simulated publication failure')
                return rename(path, target)

            with patch.object(pathlib.Path, 'rename', fail_incoming):
                with self.assertRaisesRegex(OSError, 'simulated'):
                    build.publish(source, destination)
            self.assertEqual((destination / 'OShell Desktop.exe').read_bytes(), b'old-program')


if __name__ == '__main__':
    unittest.main()

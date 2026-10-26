"""Verify source-runtime packaging restores manifests even on build failure."""
import hashlib
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import terminal_runtime as runtime


class RuntimeTests(unittest.TestCase):
    def test_manifest_and_content_cache_are_transactional(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            manifest = root / 'internal/terminal/mygo-plugin.json'
            manifest.parent.mkdir(parents=True)
            original = b'{"libraries":[{"name":"libghostty-vt","files":{}}]}'
            manifest.write_bytes(original)
            binary = root / 'fixture.dll'
            binary.write_bytes(b'synthetic-native-library')
            sha = hashlib.sha256(binary.read_bytes()).hexdigest()
            with patch.object(runtime, 'ROOT', root), patch.object(runtime, 'prepare', return_value=binary), \
                 patch.object(runtime.platform, 'system', return_value='Windows'), \
                 patch.dict(os.environ, {'LOCALAPPDATA': str(root / 'cache')}):
                with self.assertRaisesRegex(RuntimeError, 'synthetic failure'):
                    with runtime.native_manifest('windows/amd64'):
                        value = json.loads(manifest.read_bytes())['libraries'][0]['files']['windows-amd64']
                        self.assertEqual(value['sha256'], sha)
                        self.assertEqual((root / 'cache/mygo/natives' / sha / 'ghostty-vt.dll').read_bytes(), binary.read_bytes())
                        raise RuntimeError('synthetic failure')
            self.assertEqual(manifest.read_bytes(), original)


if __name__ == '__main__':
    unittest.main()

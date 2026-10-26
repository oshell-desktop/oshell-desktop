"""Build the pinned Ghostty runtime with OShell's per-terminal width policy."""
from contextlib import contextmanager
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parent.parent
COMMIT = 'befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028'
ZIG_VERSION = '0.16.0'
ZIG_HASHES = {
    'windows': '68659eb5f1e4eb1437a722f1dd889c5a322c9954607f5edcf337bc3684a75a7e',
    'linux': '70e49664a74374b48b51e6f3fdfbf437f6395d42509050588bd49abe52ba3d00',
}
TARGETS = {'windows/amd64': ('x86_64-windows-gnu', 'bin/ghostty-vt.dll'),
           'linux/amd64': ('x86_64-linux-gnu.2.28', 'lib/libghostty-vt.so')}


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def cache_root():
    override = os.environ.get('OSHELL_TERMINAL_CACHE')
    if override:
        return Path(override).resolve()
    if os.environ.get('XDG_CACHE_HOME'):
        return Path(os.environ['XDG_CACHE_HOME']) / 'oshell-terminal'
    return ROOT / 'validation/vt'


def run(*args, cwd=None):
    subprocess.run(list(map(str, args)), cwd=cwd, check=True)


def toolchain(cache):
    if override := os.environ.get('OSHELL_ZIG'):
        zig = Path(override).resolve()
        if subprocess.check_output([str(zig), 'version'], text=True).strip() != ZIG_VERSION:
            raise RuntimeError('OSHELL_ZIG must name Zig ' + ZIG_VERSION)
        return zig
    host = platform.system().lower()
    if host not in ZIG_HASHES or platform.machine().lower() not in ('amd64', 'x86_64'):
        raise RuntimeError('The pinned terminal compiler supports Windows/Linux x64 hosts.')
    name = f'zig-x86_64-{host}-{ZIG_VERSION}'
    zig = cache / name / ('zig.exe' if host == 'windows' else 'zig')
    if not zig.exists():
        cache.mkdir(parents=True, exist_ok=True)
        extension = '.zip' if host == 'windows' else '.tar.xz'
        archive = cache / (name + extension)
        if not archive.exists() or digest(archive) != ZIG_HASHES[host]:
            temporary = archive.with_suffix('.download')
            urllib.request.urlretrieve(f'https://ziglang.org/download/{ZIG_VERSION}/{name}{extension}', temporary)
            if digest(temporary) != ZIG_HASHES[host]:
                raise RuntimeError('Zig archive checksum mismatch')
            os.replace(temporary, archive)
        if host == 'windows':
            with zipfile.ZipFile(archive) as z:
                z.extractall(cache)
        else:
            with tarfile.open(archive) as z:
                z.extractall(cache, filter='data')
    return zig


def prepare(target):
    if target not in TARGETS:
        raise ValueError('Unsupported terminal target: ' + target)
    patch = ROOT / 'third_party/ghostty/oshell.patch'
    width = ROOT / 'third_party/ghostty/oshell_width.zig'
    selection = ROOT / 'third_party/ghostty/oshell_selection.zig'
    fingerprint = hashlib.sha256((COMMIT + ZIG_VERSION + platform.system() + target + digest(patch) + digest(width) + digest(selection)).encode()).hexdigest()
    cache = cache_root()
    work = cache / fingerprint[:12]
    binary = work / 'install' / TARGETS[target][1]
    receipt = work / 'built.json'
    if receipt.exists() and binary.exists():
        if json.loads(receipt.read_text(encoding='utf-8')).get('sha256') == digest(binary):
            return binary
    zig = toolchain(cache)
    work.mkdir(parents=True, exist_ok=True)
    source = work / 's'
    if not source.exists():
        run('git', 'init', str(source))
        run('git', '-c', 'core.autocrlf=false', '-C', source, 'fetch', '--depth=1', 'https://github.com/ghostty-org/ghostty.git', COMMIT)
        run('git', '-c', 'core.autocrlf=false', '-C', source, '-c', 'core.longpaths=true', 'checkout', '--detach', 'FETCH_HEAD')
    revision = subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip()
    if revision != COMMIT:
        raise RuntimeError('Terminal source revision mismatch')
    applied = source / '.oshell-patched'
    if not applied.exists():
        run('git', 'apply', '--check', str(patch), cwd=source)
        run('git', 'apply', str(patch), cwd=source)
        shutil.copy2(width, source / 'src/terminal/oshell_width.zig')
        shutil.copy2(selection, source / 'src/terminal/oshell_selection.zig')
        applied.write_text(fingerprint, encoding='utf-8')
    # Exclude upstream helper Go files from the application's ./... tests.
    (source / 'go.mod').write_text('module ghostty-build-source\n\ngo 1.27.1\n', encoding='utf-8')
    run(zig, 'build', '-Demit-lib-vt=true', '-Doptimize=ReleaseFast', '-Dsimd=false',
        '-Dtarget=' + TARGETS[target][0], '--prefix', work / 'install', cwd=source)
    if not binary.is_file():
        raise RuntimeError('Terminal build did not produce ' + str(binary))
    receipt.write_text(json.dumps({'sha256': digest(binary), 'commit': COMMIT, 'patch': fingerprint}), encoding='utf-8')
    return binary


@contextmanager
def native_manifest(target):
    lock = ROOT / 'validation/terminal-build.lock'
    lock.parent.mkdir(parents=True, exist_ok=True)
    try:
        handle = lock.open('x', encoding='utf-8')
    except FileExistsError as error:
        raise RuntimeError('A terminal package build is already in progress; inspect validation/terminal-build.lock.') from error
    try:
        handle.write(str(os.getpid()))
        handle.close()
        with prepared_manifest(target) as binary:
            yield binary
    finally:
        handle.close()
        lock.unlink()


@contextmanager
def prepared_manifest(target):
    """Use the verified source build for packaging; always restore the source manifest."""
    binary = prepare(target)
    sha = digest(binary)
    # mygo's content-addressed cache is also checked on every build.
    if platform.system() == 'Windows':
        base = Path(os.environ['LOCALAPPDATA'])
    else:
        base = Path(os.environ.get('XDG_CACHE_HOME', str(Path.home() / '.cache')))
    name = 'ghostty-vt.dll' if target.startswith('windows/') else 'libghostty-vt.so'
    cached = base / 'mygo/natives' / sha / name
    cached.parent.mkdir(parents=True, exist_ok=True)
    if not cached.exists() or digest(cached) != sha:
        shutil.copy2(binary, cached)
    path = ROOT / 'internal/terminal/mygo-plugin.json'
    original = path.read_bytes()
    doc = json.loads(original)
    entry = next(lib for lib in doc['libraries'] if lib['name'] == 'libghostty-vt')
    entry['files'][target.replace('/', '-')] = {'name': name, 'sha256': sha, 'url': 'source-build://ghostty/' + COMMIT}
    try:
        path.write_text(json.dumps(doc, indent=2) + '\n', encoding='utf-8')
        yield binary
    finally:
        path.write_bytes(original)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--target', choices=TARGETS, required=True)
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    binary = prepare(args.target)
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if command:
        env = dict(os.environ, MYGO_GHOSTTY_VT=str(binary), OSHELL_REQUIRE_WIDTH_EXTENSION='1')
        subprocess.run(command, env=env, check=True)
    else:
        print(binary)

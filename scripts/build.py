#!/usr/bin/env python3
"""Build a native app locally; add --archive for GitHub Actions artifacts."""
import argparse
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import tarfile
import zipfile
from single_file import embedded_resources
from terminal_runtime import native_manifest

ROOT = Path(__file__).resolve().parent.parent
TARGETS = ('windows/amd64', 'linux/amd64')


def run(*args, env=None):
    subprocess.run(args, cwd=ROOT, env=env, check=True)


def prepare_linux_archive(product):
    """Restore Unix execute bits lost when mygo packages on Windows."""
    archives = list(product.glob('*.tar.gz'))
    if len(archives) != 1:
        raise RuntimeError('Expected exactly one Linux runtime archive')
    archive = archives[0]
    temporary = archive.with_suffix('.tmp')
    try:
        with tarfile.open(archive, 'r:gz') as source, tarfile.open(temporary, 'w:gz') as out:
            for member in source:
                if member.isdir() or member.name == 'oshell-desktop':
                    member.mode = 0o755
                if member.isfile():
                    with source.extractfile(member) as contents:
                        out.addfile(member, contents)
                else:
                    out.addfile(member)
        with tarfile.open(temporary, 'r:gz') as check:
            for name in ('oshell-desktop', 'libghostty-vt.so'):
                if not check.getmember(name).isfile():
                    raise RuntimeError('Missing Linux runtime file: ' + name)
        os.replace(temporary, archive)
    finally:
        temporary.unlink(missing_ok=True)


def publish(source, destination):
    """Stage the complete result before replacing the previous build."""
    if ((source / 'oshell-portable').is_file() or (source / 'OShell Desktop.exe').is_file()) and destination.exists():
        # Portable apps store credentials beside the executable. Rebuilding
        # must neither delete those files nor include them in a release ZIP.
        incoming_files = {p.relative_to(source) for p in source.rglob('*') if p.is_file()}
        for existing in destination.rglob('*'):
            if existing.is_file() and existing.relative_to(destination) not in incoming_files:
                raise RuntimeError('Portable output contains local data or extra files; choose a clean --out directory.')
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.oshell-publish-', dir=destination.parent) as folder:
        incoming = Path(folder) / destination.name
        shutil.copytree(source, incoming)
        backup = Path(folder) / 'previous'
        had_previous = destination.exists()
        if had_previous:
            destination.rename(backup)
        try:
            incoming.rename(destination)
        except OSError:
            if had_previous:
                backup.rename(destination)
            raise


def build(target, output, archive=False, single_file=False):
    if target not in TARGETS:
        raise SystemExit('Supported build targets: ' + ', '.join(TARGETS))
    goos, arch = target.split('/')
    if single_file and target != 'windows/amd64':
        raise SystemExit('Single-file packaging supports Windows amd64 only.')
    with native_manifest(target):
        return build_prepared(target, output, archive, single_file)


def build_prepared(target, output, archive=False, single_file=False):
    goos, arch = target.split('/')
    generated = ROOT / 'mygo.ts'
    previous = generated.read_bytes() if generated.exists() else None
    with tempfile.TemporaryDirectory(prefix='oshell-build-') as folder:
        stage = Path(folder)
        try:
            run('go', 'tool', 'mygo', 'build', '-platform', target,
                '-skip-build-command', '-skip-dmg', '-o', str(stage))
            if single_file:
                # First collect exactly the resources selected by mygo, then
                # rebuild with those bytes embedded. Both outputs are clean
                # temporary builds, never a user's portable data directory.
                with embedded_resources(ROOT, stage / 'windows-amd64'):
                    if os.environ.get('OSHELL_CI_TEST_BUNDLE') == '1':
                        run('python3', 'scripts/ci-test-bundle.py', 'embedded')
                    if platform.system() == 'Windows':
                        run('go', 'test', '-count=1', '-timeout=60s', './internal/terminal',
                            '-run', '^TestSingleFileResourcesLoad$')
                        run('go', 'test', '-count=1', '-timeout=60s', './internal/core',
                            '-run', '^TestSingleFileStorageActivation$')
                    run('go', 'tool', 'mygo', 'build', '-platform', target,
                        '-skip-build-command', '-skip-dmg', '-o', str(stage / 'embedded'))
                single = stage / 'single-file'
                single.mkdir()
                shutil.copy2(stage / 'embedded/windows-amd64/OShell Desktop.exe', single / 'OShell Desktop.exe')
        finally:
            # This native app has no TS client. Do not leave generated stubs.
            if previous is None:
                generated.unlink(missing_ok=True)
            else:
                generated.write_bytes(previous)
        platform_dir = stage / f'{goos}-{arch}'
        if single_file:
            product = stage / 'single-file'
            destination = output / 'windows-amd64-single-file'
        elif goos == 'linux':
            product = platform_dir
            destination = output / f'{goos}-{arch}'
            prepare_linux_archive(product)
        else:
            product = platform_dir
            destination = output / f'{goos}-{arch}'
            for installer in product.glob('OShell Desktop Setup *.exe'):
                installer.unlink()
            (product / '使用说明.txt').write_text('Windows amd64 绿色便携版\n完整解压后直接双击 OShell Desktop.exe。无需安装、启动脚本或管理员权限。\n配置、凭据、主机指纹等数据文件保存在 EXE 同级目录。移动时复制整个文件夹。\n请保留 oshell-portable 标记和 ghostty-vt.dll，并放在可写目录。\n旧版配置请先关闭旧版，再将原数据目录的完整数据文件复制过来；不要覆盖程序文件。\n更新时请先完全退出旧版，只替换程序文件，保留原配置、凭据、密钥及 state-transaction.json；新版启动会自动恢复未完成的保存。\n', encoding='utf-8-sig')
            (product / 'oshell-portable').write_text('OShell portable mode: data stays beside the executable.\n', encoding='utf-8')

        publish(product, destination)
        if archive:
            suffix = '-single-file' if single_file else ''
            extension = '.tar.gz' if goos == 'linux' else '.zip'
            zip_path = output / f'OShell-Desktop-{goos}-{arch}{suffix}{extension}'
            # Keep a previous archive until its replacement is complete.
            with tempfile.TemporaryDirectory(prefix='.oshell-archive-', dir=output) as temp:
                zipped = Path(temp) / zip_path.name
                if goos == 'linux':
                    # Reuse the runtime-only tarball, without nesting the
                    # generated Debian package and installer inside it.
                    archives = list(destination.glob('*.tar.gz'))
                    if len(archives) != 1:
                        raise RuntimeError('Expected exactly one Linux runtime archive')
                    shutil.copy2(archives[0], zipped)
                    with tarfile.open(zipped, 'r:gz') as check:
                        for member in check:
                            if member.isfile():
                                with check.extractfile(member) as contents:
                                    while contents.read(1024 * 1024):
                                        pass
                else:
                    with zipfile.ZipFile(zipped, 'w', zipfile.ZIP_DEFLATED) as out:
                        for file in sorted(destination.rglob('*')):
                            if file.is_file():
                                out.write(file, str(file.relative_to(destination.parent)))
                    with zipfile.ZipFile(zipped) as check:
                        if check.testzip() is not None:
                            raise RuntimeError('Archive verification failed')
                os.replace(zipped, zip_path)
            print(zip_path)
        print(destination)


if __name__ == '__main__':
    default_os = {'Linux': 'linux', 'Windows': 'windows'}.get(platform.system())
    default_arch = {'arm64': 'arm64', 'aarch64': 'arm64', 'x86_64': 'amd64', 'AMD64': 'amd64'}.get(platform.machine())
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--target', choices=TARGETS, default=f'{default_os}/{default_arch}')
    parser.add_argument('--out', type=Path, default=ROOT / 'dist')
    parser.add_argument('--archive', action='store_true', help='also publish a Windows ZIP or Linux tar.gz for CI')
    parser.add_argument('--single-file', action='store_true', help='embed Windows resources in one portable EXE')
    args = parser.parse_args()
    if args.target not in TARGETS:
        parser.error('Specify a supported --target (windows/amd64 or linux/amd64).')
    build(args.target, args.out.resolve(), args.archive, args.single_file)

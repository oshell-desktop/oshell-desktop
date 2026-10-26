"""Compile tests inside Docker, or execute the resulting Windows bundle without Go."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
from terminal_runtime import prepare, digest

ROOT = Path(__file__).resolve().parent.parent
BUNDLE = ROOT / 'validation/windows-tests'
CORE = 'oshell-desktop/internal/core'
RACE = [CORE, 'oshell-desktop/internal/terminal/internal/pty', 'oshell-desktop/internal/zmodem']
PARSERS = ['github.com/jlaffaye/ftp', 'github.com/pkg/sftp']
MYGO = 'github.com/egoist/mygo'
DIALOG_TESTS = {
    MYGO: '^Test(OpenContext|DialogCancellation)',
    MYGO + '/internal/windows': '^TestFileDialog',
}
TERMINAL_RACE = '^Test(TransferCancelWhilePickerBackpressures|TransferPeerCancelWhilePickerOpen|TransferSkippedBatchRestoresShellWithoutAbort|TransferMultiplexer|DisconnectCancelsTransfer)'
# The full core suite includes real 600,000-round password derivations and
# exceeds three minutes. Keep a finite budget shared by both normal suites;
# race shards retain their separate, existing budget.
REGRESSION_TIMEOUT = '8m'


def run(command, **kwargs):
    print(' '.join(map(str, command)), flush=True)
    subprocess.run(list(map(str, command)), check=True, **kwargs)


def objects(text):
    decoder = json.JSONDecoder()
    while text.strip():
        value, end = decoder.raw_decode(text.lstrip())
        yield value
        text = text.lstrip()[end:]


def compile_bundle(embedded=False):
    if not Path('/.dockerenv').exists():
        raise RuntimeError('CI compilation requires Docker')
    BUNDLE.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, GOOS='windows', GOARCH='amd64', CGO_ENABLED='0')
    manifest_path = BUNDLE / 'manifest.json'
    if embedded:
        manifest = json.loads(manifest_path.read_text())
        specs = [(CORE, False, '^TestSingleFileStorageActivation$', 'embedded-core'),
                 ('oshell-desktop/internal/terminal', False, '^TestSingleFileResourcesLoad$', 'embedded-terminal')]
    else:
        manifest = {'commit': os.environ['GITHUB_SHA'], 'tests': []}
        runtime = prepare('windows/amd64')
        shutil.copy2(runtime, BUNDLE / 'ghostty-vt.dll')
        manifest['terminalSHA256'] = digest(runtime)
        listed = subprocess.check_output(['go', 'list', '-json', './...', *PARSERS, *DIALOG_TESTS], cwd=ROOT, env=env, text=True)
        packages = [p for p in objects(listed) if p.get('TestGoFiles') or p.get('XTestGoFiles')]
        specs = [(p['ImportPath'], False, 'Test(UNIXMetadata|OwnerHints|Parse)' if p['ImportPath'] in PARSERS else DIALOG_TESTS.get(p['ImportPath'], '.'),
                  'normal-' + str(i)) for i, p in enumerate(packages)]
        specs += [(p, True, '.', 'race-' + p.rsplit('/', 1)[-1]) for p in RACE]
        specs.append(('oshell-desktop/internal/terminal', True, TERMINAL_RACE, 'race-terminal'))
        run(['go', 'build', '-o', BUNDLE / 'test2json.exe', 'cmd/test2json'], cwd=ROOT, env=env)
        run(['go', 'build', '-o', BUNDLE / 'update-fixture.exe', './internal/core/testdata/update-fixture'], cwd=ROOT, env=env)
        run(['go', 'build', './...'], cwd=ROOT, env=env)
        run(['go', 'vet', './...'], cwd=ROOT, env=env)
    for package, race, pattern, name in specs:
        test_env = dict(env)
        if race:
            test_env.update(CGO_ENABLED='1', CC='x86_64-w64-mingw32-gcc', CXX='x86_64-w64-mingw32-g++')
        binary = name + '.exe'
        run(['go', 'test', '-c', *(['-race'] if race else []), '-o', BUNDLE / binary, package], cwd=ROOT, env=test_env)
        directory = ('third_party/ftp' if package == PARSERS[0] else 'third_party/sftp' if package == PARSERS[1]
                     else package.removeprefix('oshell-desktop/'))
        if package == 'oshell-desktop':
            directory = '.'
        if package == MYGO or package.startswith(MYGO + '/'):
            directory = 'third_party/mygo' + package[len(MYGO):]
        manifest['tests'].append(dict(package=package, binary=binary, race=race, pattern=pattern, directory=directory))
    manifest_path.write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')


def execute_windows(shard):
    manifest = json.loads((BUNDLE / 'manifest.json').read_text())
    if manifest['commit'] != os.environ['GITHUB_SHA']:
        raise RuntimeError('Test bundle does not belong to this commit')
    os.environ['OSHELL_PRECOMPILED_TESTS'] = '1'
    os.environ['OSHELL_UPDATE_FIXTURE'] = str(BUNDLE / 'update-fixture.exe')
    if shard is not None:
        run([sys.executable, ROOT / 'scripts/core_test_shards.py', 'run', '--platform', 'Windows',
             '--shard', str(shard), '--binary', BUNDLE / 'race-core.exe', '--test2json', BUNDLE / 'test2json.exe'])
        return
    runtime = BUNDLE / 'ghostty-vt.dll'
    if digest(runtime) != manifest.get('terminalSHA256'):
        raise RuntimeError('Precompiled terminal runtime checksum mismatch')
    os.environ['MYGO_GHOSTTY_VT'] = str(runtime)
    os.environ['OSHELL_REQUIRE_WIDTH_EXTENSION'] = '1'
    for item in manifest['tests']:
        if item['race'] and item['package'] == CORE:
            continue
        run([BUNDLE / item['binary'], '-test.v', '-test.paniconexit0', '-test.count=1', '-test.timeout=' + REGRESSION_TIMEOUT, '-test.run', item['pattern']],
            cwd=ROOT / item['directory'])
    for script in ['test-build.py', 'test-terminal-runtime.py', 'test-configuration-contract.py', 'test-core-shards.py', 'test-ci-bundle.py']:
        run([sys.executable, ROOT / 'scripts' / script], cwd=ROOT)


def stage_windows_bundles(source=BUNDLE, output=ROOT / 'validation'):
    """Publish only the binaries consumed by each Windows job, without recompiling."""
    manifest = json.loads((source / 'manifest.json').read_text())
    groups = {'windows-regression': [], 'windows-core-race': []}
    for item in manifest['tests']:
        group = 'windows-core-race' if item['race'] and item['package'] == CORE else 'windows-regression'
        groups[group].append(item)
    if len(groups['windows-core-race']) != 1:
        raise ValueError('Expected exactly one core race binary')
    plans = {}
    for name, tests in groups.items():
        files = {item['binary'] for item in tests} | {'update-fixture.exe'}
        if name == 'windows-core-race':
            files.add('test2json.exe')
        elif manifest.get('terminalSHA256'):
            files.add('ghostty-vt.dll')
        if (output / name).exists():
            raise ValueError('Refusing to mix a stale Windows test bundle')
        for file in files:
            if Path(file).name != file or not (source / file).is_file():
                raise ValueError('Missing or invalid test binary: ' + file)
        plans[name] = (tests, files)
    for name, (tests, files) in plans.items():
        target = output / name
        target.mkdir(parents=True)
        for file in sorted(files):
            shutil.copy2(source / file, target / file)
        (target / 'manifest.json').write_text(json.dumps(dict(manifest, tests=tests), indent=2) + '\n', encoding='utf-8')
        size = sum((source / file).stat().st_size for file in files)
        print(f'{name}: {len(files)} binaries, {size} uncompressed bytes', flush=True)


def linux(shard=None):
    if not Path('/.dockerenv').exists():
        raise RuntimeError('Linux CI tests require Docker')
    os.environ['CGO_ENABLED'] = '1'
    os.environ['OSHELL_REQUIRE_LRZSZ'] = '1'
    fixture = ROOT / 'validation/linux-update-fixture'
    fixture.parent.mkdir(exist_ok=True)
    run(['go', 'build', '-o', fixture, './internal/core/testdata/update-fixture'], cwd=ROOT)
    os.environ['OSHELL_UPDATE_FIXTURE'] = str(fixture)
    if shard is not None:
        core_binary = ROOT / 'validation/linux-core.test'
        adapter = ROOT / 'validation/linux-test2json'
        run(['go', 'test', '-race', '-c', '-o', core_binary, './internal/core'], cwd=ROOT)
        run(['go', 'build', '-o', adapter, 'cmd/test2json'], cwd=ROOT)
        run([sys.executable, ROOT / 'scripts/core_test_shards.py', 'run', '--platform', 'Linux',
             '--shard', str(shard), '--binary', core_binary, '--test2json', adapter], cwd=ROOT)
        return
    os.environ['MYGO_GHOSTTY_VT'] = str(prepare('linux/amd64'))
    os.environ['OSHELL_REQUIRE_WIDTH_EXTENSION'] = '1'
    run(['go', 'test', '-race', '-count=1', '-timeout=5m', *RACE[1:]], cwd=ROOT)
    run(['go', 'test', '-race', '-count=1', '-timeout=2m', './internal/terminal', '-run', TERMINAL_RACE], cwd=ROOT)
    run(['xvfb-run', '-a', 'dbus-run-session', '--', 'go', 'test', '-count=1', '-timeout=' + REGRESSION_TIMEOUT, './...'], cwd=ROOT)
    run(['go', 'test', '-count=1', *PARSERS, '-run', 'Test(UNIXMetadata|OwnerHints|Parse)'], cwd=ROOT)
    run(['go', 'test', '-count=1', MYGO, '-run', DIALOG_TESTS[MYGO]], cwd=ROOT)
    for script in ['test-build.py', 'test-terminal-runtime.py', 'test-core-shards.py', 'test-configuration-contract.py', 'test-ci-bundle.py']:
        run([sys.executable, ROOT / 'scripts' / script], cwd=ROOT)
    run(['go', 'build', './...'], cwd=ROOT, env=dict(os.environ, CGO_ENABLED='0'))
    run(['go', 'vet', './...'], cwd=ROOT)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['compile', 'embedded', 'stage', 'windows', 'linux', 'linux-core'])
    parser.add_argument('--shard', type=int)
    args = parser.parse_args()
    if args.mode == 'stage':
        stage_windows_bundles()
    elif args.mode in ('compile', 'embedded'):
        compile_bundle(args.mode == 'embedded')
    elif args.mode == 'windows':
        execute_windows(args.shard)
    else:
        if args.mode == 'linux-core' and args.shard is None:
            parser.error('linux-core requires --shard')
        linux(args.shard)

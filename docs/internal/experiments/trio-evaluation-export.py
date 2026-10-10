#!/usr/bin/env python3
"""Export only the paired trio runner's audit artifacts, with private values removed."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import sys


CASE = re.compile(r'(payment|unicode|short)-(concise|rtk|caveman|both)-[0-9]+\Z')
PUBLIC_SKILLS = {'caveman', 'design', 'doctor', 'plugin-authoring'}
PUBLIC_AGENTS = {'general-purpose', 'Explore', 'Plan', 'statusline-setup', 'claude-code-guide'}
COMMON = set('agent agents context object design issue code tool tools skill skills plugin plugins '
             'review audit docs help test tests memory shared global local work project release '
             'creator installer author spec management runtime compress stats commit handoff '
             'claude rtk caveman payment unicode short concise both doctor private public builtin '
             'case dataset selected executable bash passed failed expected got command error path paths '
             'fixture off on café cafe timeout type name id message content input result usage model '
             'text role cwd'.split())
ID_KEYS = {'id', 'uuid', 'session_id', 'tool_use_id', 'parent_tool_use_id', 'request_id', 'hook_id'}
UUID = re.compile(r'\b[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}\b', re.I)
PREFIX_ID = re.compile(r'\b(?:toolu|msg|req|sess)_[A-Za-z0-9_-]+\b')
JSON_STRING = re.compile(r'"(?:\\.|[^"\\])*"')
PRIVATE_PATH = re.compile(r'(?<![\w])(?:/(?:Users|home|tmp|private/tmp|var/folders)/[^\r\n\t\x00\"\'<>;,|`\)\]\}]+|~/(?:[^\r\n\t\x00\"\'<>;,|`\)\]\}]+))')
SECRET = re.compile(r'(?i)(?:\b(?:sk-(?:ant-|proj-)?|gh[pousr]_|github_pat_|xox[baprs]-)[A-Za-z0-9_-]{12,}'
                    r'|\bAKIA[A-Z0-9]{16}\b|-----BEGIN (?:[A-Z ]*PRIVATE KEY)-----'
                    r'|\b(?:Bearer|Basic)\s+[A-Za-z0-9._~+/-]{12,}'
                    r'|\b(?:[a-z0-9]+[_-])*(?:api[_-]?key|access[_-]?token|auth[_-]?token|password|secret)[\"\']?\s*[:=]\s*[\"\']?[^\s\"\'<>]{6,}'
                    r'|\beyJ[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,})')
SECRET_KEYS = {'apikey', 'accesstoken', 'auth', 'authtoken', 'authorization', 'password', 'secret', 'credentials',
               'environment', 'env', 'token', 'clientsecret', 'refreshtoken', 'privatekey', 'sessiontoken'}
CASE_FILES = (
    'stdout.jsonl', 'stderr.txt', 'hook-events.jsonl', 'rtk-argv.jsonl',
    'transcript.txt', 'fixture-runs.txt', 'fixture-args.txt', 'observe.py',
    'agnostic-ai.yaml', 'generated-settings.json', 'generated-native-handler.json',
    'native-hook-command.json', 'probe-evidence.json', 'sync-stdout.txt', 'sync-stderr.txt',
    'check-stdout.txt', 'check-stderr.txt', '.claude/settings.json',
    '.agnostic-ai/AGNOSTIC_AI.md', 'CLAUDE.md', 'bin/rtk', 'bin/cargo', 'bin/fixture-report',
)
SKILL_FILES = ('SKILL.md', 'LICENSE', 'LICENSE-MIT', 'NOTICE', 'LICENSING.md', 'UPSTREAM-README.md')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


class JsonObject(dict):
    def __init__(self, pairs):
        super().__init__(pairs)
        self.pairs = pairs


def object_pairs(value):
    return value.pairs if isinstance(value, JsonObject) else list(value.items())


def known_settings_artifact(relative):
    case, separator, path = relative.partition('/')
    return bool(separator and (CASE.fullmatch(case) or case == 'noop-preflight')
                and path in ('.claude/settings.json', 'generated-settings.json'))


def check_secrets(value, allow_target_env=False):
    if isinstance(value, str):
        require(not SECRET.search(value), 'Suspected credential value; export refused without printing it')
        if value.lstrip().startswith(('{', '[')):
            try:
                nested = json.loads(value, object_pairs_hook=JsonObject)
            except ValueError:
                nested = None
            if nested is not None:
                check_secrets(nested, allow_target_env)
    elif isinstance(value, list):
        for child in value:
            check_secrets(child)
    elif isinstance(value, dict):
        for key, child in object_pairs(value):
            normalized = re.sub(r'[^a-z]', '', key.lower())
            public_target = (allow_target_env and key == 'env' and isinstance(child, dict)
                             and object_pairs(child) == [('AGNOSTIC_AI_TARGET', 'claude')])
            require(normalized not in SECRET_KEYS or child in (None, '', {}, []) or public_target,
                    'Credential or environment field present; export refused without printing it')
            check_secrets(key)
            check_secrets(child)


def read_artifact(root, relative, required=False):
    path = root / relative
    for candidate in [path, *path.parents]:
        if candidate == root.parent:
            break
        require(not candidate.is_symlink(), 'Symlink in selected audit artifact; export refused')
    if not path.exists():
        require(not required, f'Missing required audit artifact: {relative}')
        return None
    require(path.is_file() and path.stat().st_size <= 32 * 1024 * 1024,
            'Selected audit artifact is not a bounded regular file')
    data = path.read_bytes()
    allow_target_env = known_settings_artifact(relative)
    if path.suffix == '.json':
        check_secrets(data.decode(), allow_target_env)
        value = json.loads(data)
    elif path.suffix == '.jsonl':
        value = []
        for line in data.decode().splitlines():
            if not line.strip():
                continue
            check_secrets(line)
            try:
                value.append(json.loads(line))
            except ValueError:
                value.append({'type': 'unparsed_stream_line', 'text': line})
    else:
        value = data.decode()
    check_secrets(value, allow_target_env)
    return {'source': relative, 'data': data, 'value': value, 'format': path.suffix}


def read_hook_requests(root, case):
    relative = case+'/hook-requests.bin'
    path = root / relative
    for candidate in [path, *path.parents]:
        if candidate == root.parent:
            break
        require(not candidate.is_symlink(), 'Symlink in selected hook requests; export refused')
    if not path.exists():
        return None
    require(path.is_file() and path.stat().st_size <= 32 * 1024 * 1024, 'Hook requests exceed the audit bound')
    data = path.read_bytes()
    offset, records = 0, []
    while offset < len(data):
        require(offset+8 <= len(data), 'Truncated hook request frame')
        size = int.from_bytes(data[offset:offset+8], 'big')
        offset += 8
        require(0 < size <= 4 * 1024 * 1024 and offset+size <= len(data), 'Invalid hook request frame')
        raw = data[offset:offset+size]
        check_secrets(raw.decode())
        request = json.loads(raw)
        check_secrets(request)
        records.append({'request': request, 'private_original_request_sha256': digest(raw),
                        'private_original_request_bytes': size})
        offset += size
    return {'source': relative, 'data': data, 'value': records, 'format': '.jsonl',
            'destination': case+'/hook-requests.jsonl'}


def public_plugin(plugin):
    return (isinstance(plugin, dict) and str(plugin.get('source', '')).endswith('@builtin')
            and not str(plugin.get('path', '')).startswith('/')
            and any(marker in str(plugin.get('path', '')) for marker in ('builtin', 'bunfs')))


class Redactions:
    def __init__(self, source, case, manifest):
        self.identifiers = {}
        self.identities = {}
        self.paths = {}
        roots = [str(source)]
        if str(source).startswith('/private/'):
            roots.append(str(source)[8:])
        self.prefixes = [(root+'/'+case, '{case}') for root in roots] if case else []
        self.prefixes.extend((root, '{dataset}') for root in roots)
        for key, label in [('skill_source', '{selected-skill}'), ('claude_executable', '{claude-executable}'),
                           ('agnostic_executable', '{agnostic-executable}'), ('rtk_processor_executable', '{rtk-executable}'),
                           ('rtk_host_target', '{rtk-executable}')]:
            if isinstance(manifest.get(key), str):
                self.prefixes.append((manifest[key], label))
        self.prefixes.sort(key=lambda pair: -len(pair[0]))

    def identity(self, name, kind):
        if not isinstance(name, str) or not name or name in PUBLIC_SKILLS or name in PUBLIC_AGENTS:
            return
        if name not in self.identities:
            self.identities[name] = f'private-{kind}-{len(self.identities)+1:03d}'
        for word in re.findall(r'[A-Za-z][A-Za-z0-9]{3,}', name):
            if word.lower() not in COMMON and word not in self.identities:
                self.identities[word] = f'private-identity-{len(self.identities)+1:03d}'

    def identifier(self, value):
        if value not in self.identifiers:
            self.identifiers[value] = f'redacted-id-{len(self.identifiers)+1:03d}'
        return self.identifiers[value]

    def collect(self, value):
        if isinstance(value, dict):
            for key, child in value.items():
                if key in ID_KEYS and isinstance(child, str) and child:
                    self.identifier(child)
                if key in ('skillOverrides', 'enabledPlugins') and isinstance(child, dict):
                    for name in child:
                        self.identity(name, 'skill' if key == 'skillOverrides' else 'plugin')
                if key in ('skills', 'slash_commands', 'terminal_slash_commands', 'agents') and isinstance(child, list):
                    for name in child:
                        self.identity(name, 'skill' if key != 'agents' else 'agent')
                if key == 'plugins' and isinstance(child, list):
                    for plugin in child:
                        if isinstance(plugin, str):
                            self.identity(plugin, 'plugin')
                        elif isinstance(plugin, dict) and not public_plugin(plugin):
                            for field in ('name', 'source'):
                                self.identity(plugin.get(field), 'plugin')
                self.collect(child)
        elif isinstance(value, list):
            for child in value:
                self.collect(child)
        elif isinstance(value, str):
            for match in [*UUID.finditer(value), *PREFIX_ID.finditer(value)]:
                self.identifier(match.group())
            if value.startswith(('{', '[')):
                try:
                    self.collect(json.loads(value))
                except ValueError:
                    pass

    def text(self, value):
        for private, public in self.prefixes:
            value = value.replace(private, public)
        replacements = {private: public for private, public in self.identifiers.items() if len(private) >= 8}
        replacements.update({private: public for private, public in self.identities.items() if private.casefold() not in COMMON})
        if replacements:
            folded = {private.casefold(): public for private, public in replacements.items()}
            pattern = r'(?<![\w])(?:'+'|'.join(re.escape(private) for private in sorted(replacements, key=len, reverse=True))+r')(?![\w])'
            value = re.sub(pattern, lambda match: replacements.get(match.group(), folded[match.group().casefold()]), value, flags=re.I)
        def path(match):
            private = match.group().rstrip(' ')
            if private not in self.paths:
                self.paths[private] = f'{{private-path-{len(self.paths)+1:03d}}}'
            return self.paths[private]+match.group()[len(private):]
        value = PRIVATE_PATH.sub(path, value)
        return value

    def sanitize(self, value, key=None):
        if isinstance(value, dict):
            return {(self.identities.get(field, self.text(field)) if key in ('skillOverrides', 'enabledPlugins') else self.text(field)):
                    self.sanitize(child, field) for field, child in value.items()}
        if isinstance(value, list):
            return [self.sanitize(child, key) for child in value]
        if isinstance(value, str):
            if key in ID_KEYS and value:
                return self.identifier(value)
            if value in self.identities and (key in ('skills', 'slash_commands', 'terminal_slash_commands', 'agents') or value.casefold() not in COMMON):
                return self.identities[value]
            if value.startswith(('{', '[')):
                def token(match):
                    decoded = json.loads(match.group())
                    self.collect(decoded)
                    return json.dumps(self.text(decoded), ensure_ascii=False)
                return JSON_STRING.sub(token, value)
            return self.text(value)
        return value


def encoded(value, format):
    if format == '.json':
        return (json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False)+'\n').encode()
    if format == '.jsonl':
        return ''.join(json.dumps(line, ensure_ascii=False, allow_nan=False)+'\n' for line in value).encode()
    return value.encode()


def export(source, destination):
    require(not source.is_symlink(), 'Source directory cannot be a symlink')
    source = source.resolve(strict=True)
    destination = destination.absolute()
    require(not destination.exists() and not destination.is_symlink(), 'Output directory already exists; export never overwrites')
    destination = destination.resolve(strict=False)
    require(source != destination and source not in destination.parents, 'Output cannot be inside the private source')
    manifest_artifact = read_artifact(source, 'manifest.json', required=True)
    results_artifact = read_artifact(source, 'results.json', required=True)
    manifest, rows = manifest_artifact['value'], results_artifact['value']
    require(isinstance(manifest, dict) and isinstance(rows, list), 'Expected runner manifest and result rows')
    cases = []
    for row in rows:
        require(isinstance(row, dict) and isinstance(row.get('case_id'), str) and CASE.fullmatch(row['case_id']),
                'Every row must have a current runner case_id; legacy debug data is not a completed dataset')
        require(row['case_id'] not in cases, 'Duplicate case_id; export refused')
        cases.append(row['case_id'])
    payloads, index, redactions = {}, {}, {}

    def stage(artifact, value, relative=None, format=None):
        relative = relative or artifact['source']
        data = encoded(value, format or artifact['format'])
        check_secrets(value, known_settings_artifact(relative))
        payloads[relative] = data
        index[relative] = {'private_original_sha256': digest(artifact['data']),
                           'private_original_bytes': len(artifact['data']), 'sanitized_sha256': digest(data),
                           'sanitized_bytes': len(data), 'private_source_artifact': artifact['source']}

    common = Redactions(source, None, manifest)
    common.collect(manifest)
    stage(manifest_artifact, common.sanitize(manifest))
    public_rows = []
    for row, case in zip(rows, cases):
        artifacts = []
        for relative in [*CASE_FILES, *('.claude/skills/caveman/'+name for name in SKILL_FILES)]:
            artifact = read_artifact(source, case+'/'+relative)
            if artifact:
                artifacts.append(artifact)
        requests = read_hook_requests(source, case)
        if requests:
            artifacts.append(requests)
        raw_stream_available = any(item['source'] == case+'/stdout.jsonl' for item in artifacts)
        context = Redactions(source, case, manifest)
        context.collect(row)
        for artifact in artifacts:
            context.collect(artifact['value'])
        public_rows.append(context.sanitize(row))
        for artifact in artifacts:
            stage(artifact, context.sanitize(artifact['value']), artifact.get('destination'))
        redactions[case] = {'identifiers': len(context.identifiers), 'private_identity_labels': len(context.identities),
                            'private_paths': len(context.paths), 'raw_stream_available': raw_stream_available,
                            'incomplete_evidence': not raw_stream_available,
                            'prepared_only': row.get('prepared') is True}
    if isinstance(manifest.get('noop_preflight'), dict):
        context = Redactions(source, 'noop-preflight', manifest)
        artifacts = [artifact for relative in CASE_FILES
                     if (artifact := read_artifact(source, 'noop-preflight/'+relative))]
        for artifact in artifacts:
            context.collect(artifact['value'])
        for artifact in artifacts:
            stage(artifact, context.sanitize(artifact['value']))
    stage(results_artifact, public_rows)
    metadata = {'format': 'trio-paired-sanitized-v1', 'rows': len(rows), 'cases': cases,
                'hash_semantics': 'Inherited runner hashes identify private original bytes. This index separately labels sanitized file hashes; sanitizing does not remeasure provider usage or cost.',
                'redaction_scope': 'Per-case identities and identifiers; selected Caveman and public host builtins remain named. Known case/selected-tool paths retain their relations; other private paths are labels.',
                'excluded': 'All files outside the strict runner audit whitelist, including profiles, auth, environment files, databases, and RTK stores. No excluded file is opened.',
                'redactions': redactions, 'files': index}
    payloads['export-manifest.json'] = encoded(metadata, '.json')
    destination.mkdir(parents=True, exist_ok=False, mode=0o755)
    for relative, data in payloads.items():
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open('xb') as stream:
            stream.write(data)
    return {'rows': len(rows), 'files': len(payloads)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True, help='Private current-runner output directory')
    parser.add_argument('--output', type=Path, required=True, help='Fresh public directory')
    args = parser.parse_args()
    try:
        print(json.dumps(export(args.source, args.output)))
    except (OSError, ValueError) as error:
        print('Export refused: '+str(error) if isinstance(error, ValueError) else 'Export refused: filesystem operation failed', file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()

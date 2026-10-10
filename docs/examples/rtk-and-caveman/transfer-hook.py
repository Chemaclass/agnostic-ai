#!/usr/bin/env python3
"""Transfer one explicitly selected project hook without copying other settings."""
import argparse
import json
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['take', 'restore'])
    parser.add_argument('--project', type=Path, required=True)
    parser.add_argument('--backup', type=Path, required=True)
    parser.add_argument('--command', help='Exact upstream-owned hook command to take')
    args = parser.parse_args()
    settings = args.project / '.claude' / 'settings.json'
    doc = json.loads(settings.read_text())
    events = doc.setdefault('hooks', {})
    if args.action == 'take':
        if not args.command:
            parser.error('take needs --command')
        if args.backup.exists():
            parser.error(f'{args.backup}: backup already exists')
        matches = [(event, i, j) for event, groups in events.items()
                   for i, group in enumerate(groups)
                   for j, handler in enumerate(group.get('hooks', []))
                   if handler.get('command') == args.command]
        if len(matches) != 1:
            parser.error(f'expected one exact command match, found {len(matches)}')
        event, i, j = matches[0]
        group = events[event][i]
        original = dict(group, hooks=[group['hooks'][j]])
        record = {'project': str(args.project.resolve()), 'event': event,
                  'index': i, 'group': original}
        with args.backup.open('x') as backup:
            backup.write(json.dumps(record, indent=2) + '\n')
        group['hooks'].pop(j)
        if not group['hooks']:
            events[event].pop(i)
        if not events[event]:
            del events[event]
    else:
        record = json.loads(args.backup.read_text())
        if record['project'] != str(args.project.resolve()):
            parser.error('backup belongs to another project')
        restored = record['group']
        command = restored['hooks'][0]['command']
        if any(handler.get('command') == command for groups in events.values()
               for group in groups for handler in group.get('hooks', [])):
            parser.error('command already exists; refusing a duplicate')
        groups = events.setdefault(record['event'], [])
        groups.insert(min(record['index'], len(groups)), restored)
    settings.write_text(json.dumps(doc, indent=2) + '\n')
    print(f'{args.action}: {settings}')


if __name__ == '__main__':
    main()

import argparse
import json
import math
from decimal import Decimal
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('source', type=Path)
parser.add_argument('verdicts', type=Path)
parser.add_argument('mapping', type=Path)
parser.add_argument('output', type=Path)
args = parser.parse_args()
manifest = json.loads((args.source/'manifest.json').read_text())
rows = json.loads((args.source/'results.json').read_text())
mapping = json.loads(args.mapping.read_text())
verdicts = json.loads(args.verdicts.read_text())
if set(mapping) != set(verdicts):
    raise ValueError('Review labels do not match the recorded map')
if len(set(mapping.values())) != len(mapping):
    raise ValueError('Each reviewed case must have exactly one label')
accepted = {case: verdicts[label]['pass'] is True for label, case in mapping.items()}
if len(set(row['case_id'] for row in rows)) != len(rows):
    raise ValueError('Duplicate case IDs')
by_id = {row['case_id']: row for row in rows}
if set(accepted) != set(by_id):
    raise ValueError('Review does not cover every attempted case')
arms = ('concise', 'rtk', 'caveman', 'both')
metrics = ('uncached_input_tokens', 'cache_creation_input_tokens', 'cache_read_input_tokens', 'output_tokens',
           'thinking_tokens_within_output', 'cli_list_price_estimate_usd', 'elapsed_seconds')

def value(row, field):
    if row is None:
        return None
    candidate = row.get('elapsed_seconds') if field == 'elapsed_seconds' else (row.get('token_accounting') or {}).get(field)
    if isinstance(candidate, bool) or not isinstance(candidate, (int, float)) or not math.isfinite(candidate):
        return None
    return Decimal(str(candidate))

def qualified(row):
    return bool(row and row.get('correct') is True and accepted.get(row['case_id']) is True
                and row.get('comparison_valid') is True
                and (row.get('token_accounting') or {}).get('measurement_complete') is True)

def public_number(number):
    return float(number) if number is not None else None

pairs = []
for task in manifest['selected_tasks']:
    for repetition in range(manifest['repetitions']):
        baseline = by_id.get(f'{task}-concise-{repetition}')
        for arm in arms[1:]:
            candidate = by_id.get(f'{task}-{arm}-{repetition}')
            deltas = {}
            for metric in metrics:
                base, other = value(baseline, metric), value(candidate, metric)
                deltas[metric] = public_number(other-base) if base is not None and other is not None else None
            pairs.append({'task': task, 'repetition': repetition, 'arm': arm,
                          'baseline_attempted': baseline is not None, 'candidate_attempted': candidate is not None,
                          'quality_and_measurement_qualified': qualified(baseline) and qualified(candidate),
                          'candidate_minus_concise': deltas})

totals = {}
for arm in arms:
    attempts = [row for row in rows if row['arm'] == arm]
    totals[arm] = {'planned': len(manifest['selected_tasks'])*manifest['repetitions'], 'attempted': len(attempts),
                   'independently_correct': sum(accepted[row['case_id']] for row in attempts),
                   'qualified': sum(qualified(row) for row in attempts), 'metrics': {}}
    for metric in metrics:
        values = [value(row, metric) for row in attempts]
        known = [number for number in values if number is not None]
        subtotal = sum(known, Decimal(0))
        totals[arm]['metrics'][metric] = {'known_attempt_subtotal': public_number(subtotal),
                                          'unknown_attempt_count': len(values)-len(known),
                                          'all_attempt_total': public_number(subtotal) if len(values) == len(known) else None}

result = {'metric_direction': 'candidate minus concise; negative means less of that recorded metric',
          'cost_basis': 'CLI list-price estimate, not billed spend', 'planned_pairs': pairs,
          'all_attempt_arm_totals': totals, 'statistical_or_general_savings_claim': False}
with args.output.open('x') as stream:
    json.dump(result, stream, ensure_ascii=False, indent=2, allow_nan=False)
    stream.write('\n')
print(json.dumps({'planned_pair_slots': len(pairs), 'attempted_cases': len(rows)}))

"""Pure wire-result parser and authored negative fixtures; no process launch."""
import json


def parse_result(transport, raw):
    if transport != 0: raise ValueError('transport failure')
    lines = raw.splitlines(keepends=True)
    if len(lines) != 2 or any(not line.endswith(b'\n') for line in lines):
        raise ValueError('incomplete result')
    header = lines[0][:-1].split(b' ')
    if len(header) != 3 or header[0] != b'execsafe-host/v1' or not header[2].isdigit():
        raise ValueError('bad header')
    try:
        body = json.loads(lines[1].decode('utf-8'))
        kind = header[1].decode('ascii'); code = int(header[2])
    except (ValueError, UnicodeError) as exc:
        raise ValueError('bad result') from exc
    if not isinstance(body, dict) or body.get('protocol') != 'execsafe-host/v1': raise ValueError('bad protocol')
    if kind not in ('root_exit','cancelled','infra_error') or body.get('kind') != kind: raise ValueError('bad kind')
    if type(body.get('exit_code')) is not int or body['exit_code'] != code or not 0 <= code <= 0xffffffff: raise ValueError('bad code')
    return body


def fixture_results():
    valid = b'execsafe-host/v1 root_exit 125\n{"protocol":"execsafe-host/v1","kind":"root_exit","exit_code":125}\n'
    assert parse_result(0, valid)['exit_code'] == 125
    for transport, raw in ((125, valid),(0,b''),(0,valid[:-1]),(0,valid.split(b'\n')[0]+b'\n'),(0,b'bad\n{}\n'),(0,valid.replace(b'125\n',b'0\n',1)),(0,valid+b'extra\n')):
        try: parse_result(transport, raw)
        except ValueError: continue
        raise AssertionError('invalid result accepted')

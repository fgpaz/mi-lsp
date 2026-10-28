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
    if kind == 'cancelled' and (body.get('signal'), code) not in (('INT',130),('TERM',143)):
        raise ValueError('bad cancellation')
    if kind == 'infra_error' and code != 125: raise ValueError('bad infrastructure code')
    if kind == 'root_exit':
        if 'signal' in body or 'error' in body: raise ValueError('bad root exit fields')
        if 'root_exit_code' in body and (type(body['root_exit_code']) is not int or body['root_exit_code'] != code):
            raise ValueError('bad root exit code')
    return body


def fixture_results():
    valid = b'execsafe-host/v1 root_exit 125\n{"protocol":"execsafe-host/v1","kind":"root_exit","exit_code":125}\n'
    assert parse_result(0, valid)['exit_code'] == 125
    for kind, code, extra in (('cancelled',130,{'signal':'INT'}),('cancelled',143,{'signal':'TERM'}),('infra_error',125,{}),('root_exit',125,{'root_exit_code':125})):
        body = {'protocol':'execsafe-host/v1','kind':kind,'exit_code':code,**extra}
        raw = ('execsafe-host/v1 %s %d\n'%(kind,code)).encode() + json.dumps(body).encode() + b'\n'
        assert parse_result(0, raw) == body
    inconsistent = (('cancelled',130,{}),('cancelled',143,{'signal':'INT'}),('cancelled',130,{'signal':'TERM'}),('cancelled',125,{'signal':'INT'}),('infra_error',1,{}),('root_exit',125,{'signal':'INT'}),('root_exit',125,{'error':'failure'}),('root_exit',125,{'root_exit_code':126}),('root_exit',125,{'root_exit_code':True}))
    for kind, code, extra in inconsistent:
        body = {'protocol':'execsafe-host/v1','kind':kind,'exit_code':code,**extra}
        raw = ('execsafe-host/v1 %s %d\n'%(kind,code)).encode() + json.dumps(body).encode() + b'\n'
        try: parse_result(0, raw)
        except ValueError: continue
        raise AssertionError('inconsistent result accepted')
    for transport, raw in ((125, valid),(0,b''),(0,valid[:-1]),(0,valid.split(b'\n')[0]+b'\n'),(0,b'bad\n{}\n'),(0,valid.replace(b'125\n',b'0\n',1)),(0,valid+b'extra\n')):
        try: parse_result(transport, raw)
        except ValueError: continue
        raise AssertionError('invalid result accepted')

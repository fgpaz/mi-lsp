// Benchmark-only opt-in extension. Never installed as a product hook.
import { Type } from '@earendil-works/pi-ai';
import { defineTool, type ExtensionAPI } from '@earendil-works/pi-coding-agent';
import { realpathSync, existsSync, appendFileSync } from 'node:fs';
import { resolve, dirname, relative, isAbsolute } from 'node:path';
import { spawn } from 'node:child_process';

export default function (pi: ExtensionAPI) {
  const root = realpathSync(process.cwd());
  const audit = process.env.BENCH_AUDIT!;
  const record = (x: object) => appendFileSync(audit, JSON.stringify(x) + '\n');
  let turns = 0;
  pi.on('turn_start', (_event, ctx) => {
    if (++turns > 12) { record({type:'limit', reason:'turn_limit'}); ctx.abort(); }
  });
  pi.on('before_provider_request', (event) => {
    // Both authorized models use these documented provider payload forms.
    const p = event.payload as Record<string, unknown>;
    if ('max_completion_tokens' in p) p.max_completion_tokens = 2048;
    else p.max_tokens = 2048;
    record({type:'request', output_cap:2048});
    return p;
  });
  pi.on('after_provider_response', (event) => record({type:'http', status:event.status}));
  pi.on('tool_call', (event) => {
    if (event.toolName === 'bench_command') return;
    if (!['read','edit','write'].includes(event.toolName)) return {block:true,reason:'benchmark_tool_denied'};
    const raw = String(event.input.path ?? '');
    const path = resolve(root, raw);
    const rel = relative(root, path);
    if (isAbsolute(rel) || !rel.startsWith('src/') || !rel.endsWith('.go'))
      return {block:true,reason:'benchmark_path_denied'};
    const actual = existsSync(path) ? realpathSync(path) : resolve(realpathSync(dirname(path)), path.split('/').pop()!);
    if (actual !== path) return {block:true,reason:'benchmark_symlink_denied'};
    if (event.toolName !== 'read' && !['src/greeter.go','src/handler.go'].includes(rel))
      return {block:true,reason:'benchmark_write_denied'};
  });
  const commands: Record<string,string[]> = {
    "mi-lsp find-files '*greet*' --format json": [process.env.BENCH_MI_LSP!, 'find-files','*greet*','--format','json'],
    'rg --files src -g *greet*': ['rg','--files','src','-g','*greet*'],
    'mi-lsp nav diagnostics src/handler.go --format json --no-daemon': [process.env.BENCH_MI_LSP!, 'nav','diagnostics','src/handler.go','--format','json','--no-daemon'],
    'gopls check src/handler.go': ['gopls','check','src/handler.go'],
  };
  pi.registerTool(defineTool({
    name:'bench_command', label:'Benchmark command',
    description:'Run exactly one allowed read-only command, without shell. Allowed: '+Object.keys(commands).join(' ; '),
    parameters:Type.Object({command:Type.String()}),
    async execute(_id, params, signal) {
      const argv = commands[params.command];
      if (!argv) throw new Error('benchmark_command_denied');
      const prefix = ['--die-with-parent','--unshare-net','--ro-bind','/','/','--bind',root,root,
        '--tmpfs','/tmp','--proc','/proc','--dev','/dev','--chdir',root,
        '--setenv','HOME',root+'/.home','--setenv','XDG_CACHE_HOME',root+'/.cache',
        '--setenv','GOCACHE',root+'/.cache/go-build','--setenv','GOMODCACHE',root+'/.cache/mod',
        '--setenv','GOPROXY','off','--setenv','GOTOOLCHAIN','local',
        '--setenv','MI_LSP_CLIENT_NAME','pi-benchmark','--setenv','MI_LSP_SESSION_ID','paired-pilot'];
      const result = await new Promise<{code:number|null,text:string}>((accept,reject) => {
        const child = spawn('bwrap', [...prefix,'--',...argv], {signal, stdio:['ignore','pipe','pipe']});
        let text='';
        child.stdout.on('data', b=>text+=b.toString()); child.stderr.on('data', b=>text+=b.toString());
        child.on('error',reject); child.on('close',code=>accept({code,text}));
      });
      return {content:[{type:'text',text:JSON.stringify(result)}], details:{command:params.command,exitCode:result.code}, isError:result.code!==0};
    }
  }));
}

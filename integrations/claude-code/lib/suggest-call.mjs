import { buildSuggestArgv, runCommand, suggestSpawnPlan, suggestTimeoutMs } from "./exec.mjs";
import { parseSuggestHint } from "./parse-suggest.mjs";

export async function callSuggest(input, { env = process.env, execImpl = runCommand, binResolver, cwd } = {}) {
  try {
    const argv = buildSuggestArgv(input);
    const plan = binResolver ? { bin: binResolver(env), args: argv } : suggestSpawnPlan(env, argv);
    const outcome = await execImpl(plan.bin, plan.args, {
      cwd: cwd || process.cwd(),
      timeoutMs: suggestTimeoutMs(env),
      env,
    });
    if (!outcome || outcome.spawnFailed || outcome.timedOut || outcome.code !== 0) return null;
    return parseSuggestHint(outcome.stdout);
  } catch {
    return null;
  }
}

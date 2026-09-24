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
    const text = String(outcome.stdout || "").replace(/^\uFEFF/, "").trim();
    if (!text) return "";
    const hint = parseSuggestHint(outcome.stdout);
    if (hint) return hint;
    if (text.startsWith("{")) {
      try {
        const parsed = JSON.parse(text);
        if (parsed && Array.isArray(parsed.items) && parsed.items.length === 0) return "";
      } catch {
        return null;
      }
    }
    return null;
  } catch {
    return null;
  }
}

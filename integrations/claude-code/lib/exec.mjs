// Argv-only spawn of the mi-lsp binary. shell is always false so a prompt
// cannot break out of its argument slot. MI_LSP_BIN must be the real
// executable (.exe on Windows); a .cmd/.bat shim fails this spawn and the
// caller fails open.
import { spawn } from "node:child_process";

const MAX_RAW_BYTES = 64_000;
const DEFAULT_TIMEOUT_MS = 1200;

export function resolveBinary(env = process.env) {
  const override = String(env.MI_LSP_BIN || "").trim();
  if (override) return override;
  return "mi-lsp";
}

export function suggestTimeoutMs(env = process.env) {
  const parsed = Number(env.MI_LSP_SUGGEST_TIMEOUT_MS);
  if (Number.isFinite(parsed) && parsed > 0) return parsed;
  return DEFAULT_TIMEOUT_MS;
}

/**
 * Argv for the real CLI: `nav suggest --format json [--tool NAME --args JSON]`.
 * A user prompt has no Read/Grep/Glob payload, so it stays a bare suggest call.
 * Tool arguments travel as one JSON element, never as a shell string.
 * @param {{tool?: string, args?: object}} input
 * @returns {string[]}
 */
export function buildSuggestArgv(input = {}) {
  const argv = ["nav", "suggest", "--format", "json"];
  const tool = String(input.tool || "").trim();
  if (tool) {
    argv.push("--tool", tool);
    argv.push("--args", JSON.stringify(input.args && typeof input.args === "object" ? input.args : {}));
  }
  return argv;
}

// Test-only seam. Production leaves MI_LSP_NODE_FIXTURE unset.
export function suggestSpawnPlan(env, argv) {
  const fixture = String(env.MI_LSP_NODE_FIXTURE || "").trim();
  if (fixture) return { bin: process.execPath, args: [fixture, ...argv] };
  return { bin: resolveBinary(env), args: argv };
}

/**
 * @param {string} bin
 * @param {string[]} args
 * @param {{cwd?: string, timeoutMs?: number, env?: NodeJS.ProcessEnv}} [opts]
 */
export function runCommand(bin, args, opts = {}) {
  const { cwd, timeoutMs = DEFAULT_TIMEOUT_MS, env = process.env } = opts;
  return new Promise((resolve) => {
    let child;
    try {
      child = spawn(bin, args, { cwd, env, shell: false, windowsHide: true });
    } catch (error) {
      resolve(failed(error));
      return;
    }

    let stdout = "";
    let stderr = "";
    let settled = false;
    let timedOut = false;
    let spawnFailed = false;

    const finish = (code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      clearTimeout(killFallback);
      resolve({ stdout, stderr, code, timedOut, spawnFailed });
    };

    const timer = setTimeout(() => {
      timedOut = true;
      try { child.kill(); } catch { /* already gone */ }
    }, Math.max(1, timeoutMs));

    const killFallback = setTimeout(() => {
      if (!settled && timedOut) finish(null);
    }, Math.max(1, timeoutMs) + 500);

    child.on("error", (error) => {
      spawnFailed = true;
      stderr += String(error?.message || error);
    });
    child.stdout?.on("data", (chunk) => {
      if (stdout.length < MAX_RAW_BYTES) stdout += chunk.toString("utf8");
    });
    child.stderr?.on("data", (chunk) => {
      if (stderr.length < MAX_RAW_BYTES) stderr += chunk.toString("utf8");
    });
    child.on("close", (code) => finish(code));
  });
}

function failed(error) {
  return {
    stdout: "",
    stderr: String(error?.message || error),
    code: null,
    timedOut: false,
    spawnFailed: true,
  };
}

#!/usr/bin/env node
// PostToolUse: count consecutive Read/Grep/Glob calls. At 3, one advisory
// that contains `mi-lsp nav suggest`, then back off until a mi-lsp tool or
// CLI call resets the counter. Other tools do not reset it.
import { pathToFileURL } from "node:url";
import { runHookMain } from "../lib/hook-main.mjs";
import { callSuggest } from "../lib/suggest-call.mjs";
import { readState, writeState } from "../lib/state.mjs";

export const RAW_THRESHOLD = 3;
const RAW_TOOLS = new Set(["Read", "Grep", "Glob"]);

export function classifyToolCall(toolName, toolInput) {
  if (RAW_TOOLS.has(toolName)) return "raw";
  const name = String(toolName || "");
  if (/mi[-_]?lsp/i.test(name)) return "milsp";
  if (toolName === "Bash" || toolName === "PowerShell") {
    const command = String(toolInput?.command || "");
    if (/\bmi-lsp\b/.test(command)) return "milsp";
  }
  return "other";
}

export function buildAdvisory(hint) {
  const head = `Van ${RAW_THRESHOLD} lecturas crudas seguidas (Read/Grep/Glob).`;
  if (hint && !/^mi-lsp\s+nav\s+suggest\b/.test(hint)) {
    return `${head} Sugerido: ${hint}. Consultá mi-lsp nav suggest.`;
  }
  return `${head} Consultá mi-lsp nav suggest.`;
}

export async function run(payload, deps = {}) {
  try {
    const env = deps.env || process.env;
    const sessionId = payload?.session_id || "default";
    const state = readState(sessionId, env);
    const kind = classifyToolCall(payload?.tool_name, payload?.tool_input);

    if (kind === "milsp") {
      state.consecutiveRaw = 0;
      state.advised = false;
      writeState(sessionId, state, env);
      return { continue: true };
    }

    if (kind !== "raw") {
      return { continue: true };
    }

    state.consecutiveRaw = (state.consecutiveRaw || 0) + 1;
    if (state.consecutiveRaw < RAW_THRESHOLD || state.advised) {
      writeState(sessionId, state, env);
      return { continue: true };
    }

    state.advised = true;
    writeState(sessionId, state, env);
    const hint = await callSuggest(
      { tool: payload?.tool_name, args: payload?.tool_input || {} },
      { ...deps, env, cwd: payload?.cwd || process.cwd() },
    );
    return {
      continue: true,
      hookSpecificOutput: {
        hookEventName: "PostToolUse",
        additionalContext: buildAdvisory(hint),
      },
    };
  } catch {
    return { continue: true };
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  runHookMain(run);
}

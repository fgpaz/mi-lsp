#!/usr/bin/env node
// UserPromptSubmit: one `mi-lsp nav suggest` call. At most one short hint,
// and only when suggest prints a nav command or the static intent line.
// Never blocks the turn.
import { pathToFileURL } from "node:url";
import { runHookMain } from "../lib/hook-main.mjs";
import { callSuggest } from "../lib/suggest-call.mjs";

export async function run(payload, deps = {}) {
  try {
    const prompt = String(payload?.prompt ?? payload?.user_prompt ?? "");
    const hint = await callSuggest(
      { event: "user_prompt", prompt },
      { ...deps, cwd: payload?.cwd || process.cwd() },
    );
    if (!hint) return { continue: true };
    return {
      continue: true,
      hookSpecificOutput: {
        hookEventName: "UserPromptSubmit",
        additionalContext: hint,
      },
    };
  } catch {
    return { continue: true };
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  runHookMain(run);
}

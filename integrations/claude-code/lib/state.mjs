import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";

export function statePath(sessionId, env = process.env) {
  const base = env.MI_LSP_CLAUDE_STATE_DIR || path.join(os.tmpdir(), "mi-lsp-claude-code-hooks");
  const safeId = String(sessionId || "default").replace(/[^A-Za-z0-9_-]/g, "_").slice(0, 80) || "default";
  return path.join(base, `${safeId}.json`);
}

export function readState(sessionId, env = process.env) {
  try {
    const parsed = JSON.parse(readFileSync(statePath(sessionId, env), "utf8"));
    if (!parsed || typeof parsed !== "object") return emptyState();
    return {
      consecutiveRaw: Number(parsed.consecutiveRaw) || 0,
      advised: parsed.advised === true,
    };
  } catch {
    return emptyState();
  }
}

export function writeState(sessionId, state, env = process.env) {
  const file = statePath(sessionId, env);
  try {
    mkdirSync(path.dirname(file), { recursive: true });
    writeFileSync(file, JSON.stringify({
      consecutiveRaw: Number(state?.consecutiveRaw) || 0,
      advised: state?.advised === true,
    }));
  } catch {
    // Ephemeral counter. A write failure starts the next event cold.
  }
}

function emptyState() {
  return { consecutiveRaw: 0, advised: false };
}

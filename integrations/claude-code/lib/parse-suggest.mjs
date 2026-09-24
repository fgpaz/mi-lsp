// One short hint, or nothing. A hint is a single mi-lsp nav command line,
// including the static one-liner `mi-lsp nav intent "<goal>"`.
const COMMAND_LINE = /^mi-lsp\s+nav\s+\S.*$/;
const MAX_HINT_CHARS = 240;

export function parseSuggestHint(stdout) {
  const text = String(stdout || "").replace(/^\uFEFF/, "").trim();
  if (!text) return null;
  const fromJson = hintFromJson(text);
  if (fromJson) return fromJson;
  for (const rawLine of text.split(/\r?\n/)) {
    const line = stripLabel(rawLine.trim());
    if (isShortCommand(line)) return line;
  }
  return null;
}

function hintFromJson(text) {
  if (!text.startsWith("{")) return null;
  try {
    const parsed = JSON.parse(text);
    const candidate = parsed?.command ?? parsed?.hint ?? parsed?.suggested_command;
    if (typeof candidate !== "string") return null;
    const line = stripLabel(candidate.trim());
    return isShortCommand(line) ? line : null;
  } catch {
    return null;
  }
}

function stripLabel(line) {
  return line.replace(/^(?:command|hint|suggested_command)\s*:\s*/i, "");
}

function isShortCommand(line) {
  return COMMAND_LINE.test(line) && line.length <= MAX_HINT_CHARS && !line.includes("\n");
}

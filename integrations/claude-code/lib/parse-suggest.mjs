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
    const item = Array.isArray(parsed?.items) ? parsed.items[0] : null;
    const fromArgv = hintFromArgv(item?.argv);
    if (fromArgv) return fromArgv;
    const candidate = item?.command ?? parsed?.command ?? parsed?.hint ?? parsed?.suggested_command;
    if (typeof candidate !== "string") return null;
    const line = stripLabel(candidate.trim());
    const qualified = line.startsWith("nav ") ? `mi-lsp ${line}` : line;
    return isShortCommand(qualified) ? qualified : null;
  } catch {
    return null;
  }
}

function hintFromArgv(argv) {
  if (!Array.isArray(argv) || argv.length === 0) return null;
  const parts = argv.map((part) => quoteArg(String(part)));
  const line = `mi-lsp ${parts.join(" ")}`;
  return isShortCommand(line) ? line : null;
}

function quoteArg(text) {
  if (/^[\w./:\\-]+$/.test(text)) return text;
  return `"${text.replace(/"/g, '\\"')}"`;
}

function stripLabel(line) {
  return line.replace(/^(?:command|hint|suggested_command)\s*:\s*/i, "");
}

function isShortCommand(line) {
  return COMMAND_LINE.test(line) && line.length <= MAX_HINT_CHARS && !line.includes("\n");
}

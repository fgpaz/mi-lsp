// Every hook entrypoint exits 0 with {"continue": true}. Timeouts, missing
// binaries, bad JSON, and thrown errors must not block the turn.
export async function runHookMain(run, io = {}) {
  const write = io.write || ((text) => process.stdout.write(text));
  const readStdin = io.readStdin || readAllStdin;
  try {
    const raw = await readStdin();
    let payload = {};
    const trimmed = String(raw || "").trim();
    if (trimmed) {
      try {
        payload = JSON.parse(trimmed);
      } catch {
        emit(write, null);
        return;
      }
    }
    if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
      emit(write, null);
      return;
    }
    let result = null;
    try {
      result = await run(payload);
    } catch {
      emit(write, null);
      return;
    }
    emit(write, result);
  } catch {
    try { emit(write, null); } catch { /* stdout already broken */ }
  } finally {
    process.exitCode = 0;
  }
}

function emit(write, result) {
  const output = { continue: true };
  const specific = result?.hookSpecificOutput;
  const text = typeof specific?.additionalContext === "string" ? specific.additionalContext.trim() : "";
  if (text && typeof specific?.hookEventName === "string") {
    output.hookSpecificOutput = {
      hookEventName: specific.hookEventName,
      additionalContext: text,
    };
  }
  write(JSON.stringify(output));
}

async function readAllStdin() {
  let raw = "";
  for await (const chunk of process.stdin) raw += chunk;
  return raw;
}

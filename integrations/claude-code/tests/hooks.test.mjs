import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { runCommand } from "../lib/exec.mjs";
import { parseSuggestHint } from "../lib/parse-suggest.mjs";
import { runHookMain } from "../lib/hook-main.mjs";
import { run as runPrompt } from "../hooks/user-prompt-submit.mjs";
import { buildAdvisory, classifyToolCall, run as runPost } from "../hooks/post-tool-use.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const fixture = path.join(root, "tests", "fixtures", "fake-mi-lsp.mjs");
const promptHook = path.join(root, "hooks", "user-prompt-submit.mjs");
const postHook = path.join(root, "hooks", "post-tool-use.mjs");

function tempEnv(extra = {}) {
  return {
    ...process.env,
    MI_LSP_CLAUDE_STATE_DIR: mkdtempSync(path.join(os.tmpdir(), "milsp-hook-")),
    MI_LSP_NODE_FIXTURE: "",
    MI_LSP_BIN: "",
    ...extra,
  };
}

function spawnHook(script, stdin, env) {
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [script], {
      env,
      shell: false,
      windowsHide: true,
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.on("close", (code) => resolve({ code, stdout, stderr }));
    child.stdin.write(stdin);
    child.stdin.end();
  });
}

test("parseSuggestHint keeps one nav command or the static intent line", () => {
  assert.equal(parseSuggestHint('mi-lsp nav intent "how indexing works"\n'), 'mi-lsp nav intent "how indexing works"');
  assert.equal(parseSuggestHint('mi-lsp nav intent "<goal>"'), 'mi-lsp nav intent "<goal>"');
  assert.equal(parseSuggestHint('hint: mi-lsp nav route "daemon"'), 'mi-lsp nav route "daemon"');
  assert.equal(parseSuggestHint('{"command":"mi-lsp nav search \\\"token\\\""}'), 'mi-lsp nav search "token"');
  assert.equal(parseSuggestHint(""), null);
  assert.equal(parseSuggestHint("{not-json"), null);
  assert.equal(parseSuggestHint("read the file yourself"), null);
});

test("runCommand spawns argv with shell false, including metacharacters", async () => {
  const argvOut = path.join(mkdtempSync(path.join(os.tmpdir(), "milsp-argv-")), "argv.json");
  const prompt = 'a && b; rm -rf / "quoted"';
  const outcome = await runCommand(process.execPath, [fixture, "nav", "suggest", "--prompt", prompt], {
    env: { ...process.env, FAKE_MILSP_MODE: "empty", FAKE_MILSP_ARGV_OUT: argvOut },
    timeoutMs: 2000,
  });
  assert.equal(outcome.spawnFailed, false);
  assert.equal(outcome.code, 0);
  assert.deepEqual(JSON.parse(readFileSync(argvOut, "utf8")), ["nav", "suggest", "--prompt", prompt]);
});

test("UserPromptSubmit emits one hint only when suggest returns a command", async () => {
  const env = tempEnv({ MI_LSP_NODE_FIXTURE: fixture, FAKE_MILSP_MODE: "command" });
  const result = await runPrompt({ prompt: "how does indexing work", session_id: "p1" }, { env });
  assert.equal(result.continue, true);
  assert.equal(result.hookSpecificOutput.additionalContext, 'mi-lsp nav intent "how indexing works"');
  assert.equal(result.hookSpecificOutput.hookEventName, "UserPromptSubmit");
  rmSync(env.MI_LSP_CLAUDE_STATE_DIR, { recursive: true, force: true });
});

test("UserPromptSubmit forwards the static one-line nav intent hint", async () => {
  const env = tempEnv({ MI_LSP_NODE_FIXTURE: fixture, FAKE_MILSP_MODE: "static" });
  const result = await runPrompt({ prompt: "anything" }, { env });
  assert.equal(result.hookSpecificOutput.additionalContext, 'mi-lsp nav intent "<goal>"');
});

test("UserPromptSubmit stays silent on empty, garbage, and non-zero suggest", async () => {
  for (const mode of ["empty", "garbage", "exit1"]) {
    const env = tempEnv({ MI_LSP_NODE_FIXTURE: fixture, FAKE_MILSP_MODE: mode });
    const result = await runPrompt({ prompt: "anything" }, { env });
    assert.deepEqual(result, { continue: true });
  }
});

test("UserPromptSubmit fails open on timeout and a missing binary", async () => {
  const slow = tempEnv({
    MI_LSP_NODE_FIXTURE: fixture,
    FAKE_MILSP_MODE: "hang",
    MI_LSP_SUGGEST_TIMEOUT_MS: "200",
  });
  const started = Date.now();
  const timed = await runPrompt({ prompt: "slow" }, { env: slow });
  assert.deepEqual(timed, { continue: true });
  assert.ok(Date.now() - started < 2000);

  const missing = tempEnv({ MI_LSP_BIN: path.join(os.tmpdir(), "no-such-mi-lsp.exe") });
  const gone = await runPrompt({ prompt: "missing" }, { env: missing });
  assert.deepEqual(gone, { continue: true });
});

test("UserPromptSubmit passes the prompt as one argv element", async () => {
  const argvOut = path.join(mkdtempSync(path.join(os.tmpdir(), "milsp-prompt-")), "argv.json");
  const prompt = 'use && not a shell';
  const env = tempEnv({
    MI_LSP_NODE_FIXTURE: fixture,
    FAKE_MILSP_MODE: "empty",
    FAKE_MILSP_ARGV_OUT: argvOut,
  });
  await runPrompt({ prompt }, { env });
  const argv = JSON.parse(readFileSync(argvOut, "utf8"));
  assert.deepEqual(argv.slice(0, 4), ["nav", "suggest", "--event", "user_prompt"]);
  assert.equal(argv.at(-1), prompt);
});

test("classifyToolCall: raw tools, mi-lsp tools, and the mi-lsp CLI", () => {
  assert.equal(classifyToolCall("Read", {}), "raw");
  assert.equal(classifyToolCall("Grep", {}), "raw");
  assert.equal(classifyToolCall("Glob", {}), "raw");
  assert.equal(classifyToolCall("mcp__plugin_mi-lsp_mi-lsp__nav_intent", {}), "milsp");
  assert.equal(classifyToolCall("Bash", { command: "mi-lsp nav route daemon" }), "milsp");
  assert.equal(classifyToolCall("PowerShell", { command: "mi-lsp nav intent indexing" }), "milsp");
  assert.equal(classifyToolCall("Bash", { command: "rg foo src" }), "other");
  assert.equal(classifyToolCall("Edit", {}), "other");
});

test("PostToolUse advises once at 3, includes mi-lsp nav suggest, then backs off", async () => {
  const env = tempEnv({ MI_LSP_NODE_FIXTURE: fixture, FAKE_MILSP_MODE: "command" });
  const sessionId = "loop";
  let last;
  for (let i = 0; i < 2; i++) {
    last = await runPost({ cwd: root, tool_name: "Read", session_id: sessionId }, { env });
    assert.deepEqual(last, { continue: true });
  }
  last = await runPost({ cwd: root, tool_name: "Grep", session_id: sessionId }, { env });
  assert.equal(last.continue, true);
  assert.match(last.hookSpecificOutput.additionalContext, /mi-lsp nav suggest/);
  assert.match(last.hookSpecificOutput.additionalContext, /mi-lsp nav intent "how indexing works"/);
  assert.equal(last.hookSpecificOutput.hookEventName, "PostToolUse");

  const fourth = await runPost({ cwd: root, tool_name: "Glob", session_id: sessionId }, { env });
  assert.deepEqual(fourth, { continue: true });
  rmSync(env.MI_LSP_CLAUDE_STATE_DIR, { recursive: true, force: true });
});

test("PostToolUse still advises with the literal suggest command when the binary is missing", async () => {
  const env = tempEnv({ MI_LSP_BIN: path.join(os.tmpdir(), "missing-mi-lsp.exe") });
  const sessionId = "missing";
  for (let i = 0; i < 2; i++) {
    await runPost({ tool_name: "Read", session_id: sessionId }, { env });
  }
  const third = await runPost({ tool_name: "Read", session_id: sessionId }, { env });
  assert.equal(third.hookSpecificOutput.additionalContext, buildAdvisory(null));
  assert.match(third.hookSpecificOutput.additionalContext, /mi-lsp nav suggest/);
  rmSync(env.MI_LSP_CLAUDE_STATE_DIR, { recursive: true, force: true });
});

test("a mi-lsp tool or CLI call resets the counter", async () => {
  const env = tempEnv({ MI_LSP_NODE_FIXTURE: fixture, FAKE_MILSP_MODE: "static" });
  const sessionId = "reset";
  await runPost({ tool_name: "Read", session_id: sessionId }, { env });
  await runPost({ tool_name: "Read", session_id: sessionId }, { env });
  await runPost({ tool_name: "mcp__mi-lsp__nav_intent", session_id: sessionId }, { env });
  let last;
  for (const tool of ["Read", "Grep", "Glob"]) {
    last = await runPost({ tool_name: tool, session_id: sessionId }, { env });
  }
  assert.match(last.hookSpecificOutput.additionalContext, /mi-lsp nav suggest/);

  await runPost({ tool_name: "Bash", tool_input: { command: "mi-lsp nav suggest" }, session_id: sessionId }, { env });
  const afterCli = await runPost({ tool_name: "Read", session_id: sessionId }, { env });
  assert.deepEqual(afterCli, { continue: true });
  rmSync(env.MI_LSP_CLAUDE_STATE_DIR, { recursive: true, force: true });
});

test("other tools do not reset the raw streak", async () => {
  const env = tempEnv({ MI_LSP_NODE_FIXTURE: fixture, FAKE_MILSP_MODE: "empty" });
  const sessionId = "other";
  await runPost({ tool_name: "Read", session_id: sessionId }, { env });
  await runPost({ tool_name: "Edit", session_id: sessionId }, { env });
  await runPost({ tool_name: "Grep", session_id: sessionId }, { env });
  const third = await runPost({ tool_name: "Glob", session_id: sessionId }, { env });
  assert.match(third.hookSpecificOutput.additionalContext, /mi-lsp nav suggest/);
  rmSync(env.MI_LSP_CLAUDE_STATE_DIR, { recursive: true, force: true });
});

test("runHookMain fail-open: bad JSON, thrown errors, and a block decision still continue", async () => {
  const chunks = [];
  const write = (text) => chunks.push(text);
  await runHookMain(async () => ({ continue: false, decision: "block" }), {
    readStdin: async () => "{",
    write,
  });
  assert.deepEqual(JSON.parse(chunks.join("")), { continue: true });

  chunks.length = 0;
  await runHookMain(async () => { throw new Error("boom"); }, {
    readStdin: async () => "{\"prompt\":\"x\"}",
    write,
  });
  assert.deepEqual(JSON.parse(chunks.join("")), { continue: true });
  assert.equal(process.exitCode, 0);
});

test("hook entrypoints exit 0 on bad JSON and a missing binary", async () => {
  const env = tempEnv({ MI_LSP_BIN: path.join(os.tmpdir(), "missing-mi-lsp.exe") });
  const bad = await spawnHook(promptHook, "{", env);
  assert.equal(bad.code, 0);
  assert.deepEqual(JSON.parse(bad.stdout), { continue: true });

  const ok = await spawnHook(postHook, JSON.stringify({
    tool_name: "Read",
    session_id: "entry",
    cwd: root,
  }), env);
  assert.equal(ok.code, 0);
  assert.equal(JSON.parse(ok.stdout).continue, true);
  rmSync(env.MI_LSP_CLAUDE_STATE_DIR, { recursive: true, force: true });
});

import { writeFileSync } from "node:fs";

const args = process.argv.slice(2);
if (process.env.FAKE_MILSP_ARGV_OUT) {
  writeFileSync(process.env.FAKE_MILSP_ARGV_OUT, JSON.stringify(args));
}

const mode = process.env.FAKE_MILSP_MODE || "empty";
if (mode === "command") {
  process.stdout.write('mi-lsp nav intent "how indexing works"\n');
} else if (mode === "static") {
  process.stdout.write('mi-lsp nav intent "<goal>"\n');
} else if (mode === "garbage") {
  process.stdout.write("{not-json\n");
} else if (mode === "exit1") {
  process.stderr.write("unavailable\n");
  process.exit(1);
} else if (mode === "hang") {
  setInterval(() => {}, 1000);
}

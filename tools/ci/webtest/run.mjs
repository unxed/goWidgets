// Builds backends/web tests as a WebAssembly binary, serves it together with
// wasm_exec.js and index.html, runs it in headless Chromium and exits with the
// Go test binary's exit code. Used by the `web` job of CI gui; see
// docs/pixy/STATUS.md (Web W1).
//
// Usage: node run.mjs   (from tools/ci/webtest, after `npm ci` and
// `npx playwright install --with-deps chromium`)
import { execFileSync } from "node:child_process";
import { copyFileSync, mkdtempSync, readFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, "..", "..", "..");
const out = mkdtempSync(join(tmpdir(), "gowidgets-webtest-"));

const goroot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
copyFileSync(join(goroot, "lib", "wasm", "wasm_exec.js"), join(out, "wasm_exec.js"));
copyFileSync(join(here, "index.html"), join(out, "index.html"));
execFileSync("go", ["test", "-c", "-o", join(out, "web.test.wasm"), "./backends/web/"], {
  cwd: repo,
  env: { ...process.env, GOOS: "js", GOARCH: "wasm", CGO_ENABLED: "0" },
  stdio: "inherit",
});

const files = {
  "/": ["index.html", "text/html"],
  "/index.html": ["index.html", "text/html"],
  "/wasm_exec.js": ["wasm_exec.js", "text/javascript"],
  "/web.test.wasm": ["web.test.wasm", "application/wasm"],
};
const server = createServer((req, res) => {
  const entry = files[new URL(req.url, "http://localhost").pathname];
  if (!entry) {
    res.writeHead(404).end();
    return;
  }
  res.writeHead(200, { "content-type": entry[1] });
  res.end(readFileSync(join(out, entry[0])));
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const url = `http://127.0.0.1:${server.address().port}/`;

const browser = await chromium.launch();
let code = 1;
try {
  const page = await browser.newPage();
  // Go's test output arrives through console.log (wasm_exec.js fd 1/2 writes).
  page.on("console", (msg) => console.log(msg.text()));
  page.on("pageerror", (err) => console.error("pageerror:", err.message));
  await page.goto(url);
  await page.waitForFunction(() => window.__result !== undefined, null, { timeout: 120_000 });
  const result = await page.evaluate(() => window.__result);
  if (result.error !== undefined) {
    console.error("could not start the test binary:", result.error);
  } else {
    code = result.exitCode;
    console.log(`web.test.wasm exited with code ${code}`);
  }
} finally {
  await browser.close();
  server.close();
}
process.exit(code);

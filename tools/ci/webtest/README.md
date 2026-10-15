# webtest

Runs the `backends/web` tests (`GOOS=js GOARCH=wasm`) in headless Chromium, so
the Web W1 driver is exercised in a real browser and not only compiled.

`run.mjs` builds `web.test.wasm` with `go test -c`, serves it with
`wasm_exec.js` and `index.html`, and exits with the Go test binary's exit code.

CI: the `web` job in `.github/workflows/ci-gui.yml`. Locally:

    cd tools/ci/webtest && npm install && npx playwright install chromium && npm test

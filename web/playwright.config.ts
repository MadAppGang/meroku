import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, devices } from "@playwright/test";
import { E2E_ROOT_MARKER } from "./tests/e2e/support/global-teardown";

const webDir = path.dirname(fileURLToPath(import.meta.url));

// One throwaway root per run: the served project directory, the meroku binary
// and empty AWS config files live under it.
//
// The runner process evaluates this file first. MEROKU_E2E_ROOT is unset, so it
// creates the root, writes the marker file holding its own pid, and sets
// MEROKU_E2E_ROOT and MEROKU_E2E_ROOT_OWNED, which the web servers below
// inherit. Worker processes (TEST_WORKER_INDEX set) inherit them too, evaluate
// this file again, find MEROKU_E2E_ROOT set and create nothing. The global
// teardown runs in the runner and deletes the root only when
// MEROKU_E2E_ROOT_OWNED names it, the marker file is present and the root is a
// meroku-e2e-* directory directly inside the OS temp directory.
//
// A root the caller passes in MEROKU_E2E_ROOT was not created by this run. If
// the caller's environment also carries MEROKU_E2E_ROOT_OWNED, for example from
// a parent run, the runner clears it unless the marker holds the runner's own
// pid (this file evaluated twice in the same runner), so the teardown keeps
// that root.
if (!process.env.MEROKU_E2E_ROOT) {
	const root = mkdtempSync(path.join(tmpdir(), "meroku-e2e-"));
	writeFileSync(path.join(root, E2E_ROOT_MARKER), `${process.pid}\n`);
	process.env.MEROKU_E2E_ROOT = root;
	process.env.MEROKU_E2E_ROOT_OWNED = root;
} else if (
	process.env.TEST_WORKER_INDEX === undefined &&
	process.env.MEROKU_E2E_ROOT_OWNED !== undefined &&
	markerPid(process.env.MEROKU_E2E_ROOT) !== String(process.pid)
) {
	delete process.env.MEROKU_E2E_ROOT_OWNED;
}

/** The pid in a root's marker file, or "" when it has none. */
function markerPid(root: string): string {
	try {
		return readFileSync(path.join(root, E2E_ROOT_MARKER), "utf8").trim();
	} catch {
		return "";
	}
}

const apiPort = process.env.MEROKU_E2E_API_PORT ?? "18080";
const webPort = process.env.MEROKU_E2E_WEB_PORT ?? "15173";
process.env.MEROKU_E2E_API_PORT = apiPort;

const apiURL = `http://127.0.0.1:${apiPort}`;
const webURL = `http://127.0.0.1:${webPort}`;

export default defineConfig({
	testDir: "./tests/e2e",
	testMatch: "**/*.spec.ts",
	outputDir: "./test-results/artifacts",
	globalTeardown: "./tests/e2e/support/global-teardown.ts",
	// Every test shares one meroku server and one project directory, and the
	// harness resets the env YAML before each test. Two tests at once would
	// write the same file.
	fullyParallel: false,
	workers: 1,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	timeout: 60_000,
	expect: { timeout: 10_000 },
	reporter: process.env.CI
		? [["github"], ["html", { open: "never" }]]
		: [["list"]],
	use: {
		baseURL: webURL,
		trace: "on-first-retry",
		screenshot: "only-on-failure",
		locale: "en-US",
		timezoneId: "UTC",
	},
	projects: [
		{
			name: "chromium",
			use: {
				...devices["Desktop Chrome"],
				viewport: { width: 1600, height: 1000 },
			},
		},
	],
	// Never reuse a server that is already listening: it could be a meroku
	// serving a real project, and the tests write to the YAML they are given.
	webServer: [
		{
			command: "node tests/e2e/support/serve-meroku.mjs",
			cwd: webDir,
			url: `${apiURL}/api/environments`,
			reuseExistingServer: false,
			// Includes `go build` of the meroku binary.
			timeout: 180_000,
			stdout: "pipe",
			stderr: "pipe",
		},
		{
			command: `pnpm exec vite --host 127.0.0.1 --port ${webPort} --strictPort`,
			cwd: webDir,
			url: webURL,
			reuseExistingServer: false,
			timeout: 120_000,
			env: { MEROKU_API_URL: apiURL },
		},
	],
});

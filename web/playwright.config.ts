import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, devices } from "@playwright/test";

const webDir = path.dirname(fileURLToPath(import.meta.url));

// One throwaway root per run: the served project directory, the meroku binary
// and empty AWS config files live under it. The config is evaluated again in
// every worker, and workers inherit the runner's environment, so only the
// runner creates the directory.
process.env.MEROKU_E2E_ROOT ??= mkdtempSync(path.join(tmpdir(), "meroku-e2e-"));

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

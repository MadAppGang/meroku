import path from "node:path";
import { fileURLToPath } from "node:url";

/** The web/ directory, whatever the working directory of the run. */
export const webDir = path.resolve(
	path.dirname(fileURLToPath(import.meta.url)),
	"../../..",
);

/** Synthetic project files copied into the served project directory. */
export const fixtureProjectDir = path.join(
	webDir,
	"tests/e2e/fixtures/project",
);

/**
 * The run's temporary root. playwright.config.ts creates it once in the runner
 * process; workers and the web servers inherit it through the environment.
 */
export function runRoot(): string {
	const root = process.env.MEROKU_E2E_ROOT;
	if (!root) {
		throw new Error(
			"MEROKU_E2E_ROOT is not set; run the suite through playwright.config.ts",
		);
	}
	return root;
}

/** The directory meroku runs in, holding the env YAML files it reads and writes. */
export function projectDir(): string {
	return path.join(runRoot(), "project");
}

/**
 * Where screenshot evidence goes. MEROKU_E2E_SCREENSHOT_DIR overrides it, so a
 * validation run can write straight into a session folder.
 */
export function screenshotDir(): string {
	const override = process.env.MEROKU_E2E_SCREENSHOT_DIR;
	return override
		? path.resolve(override)
		: path.join(webDir, "test-results", "screenshots");
}

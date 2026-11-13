import { existsSync, realpathSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

/** The file playwright.config.ts writes into a root it created itself. */
export const E2E_ROOT_MARKER = ".meroku-e2e-root";

/**
 * Whether `root` is a directory this run created: the runner recorded it in
 * MEROKU_E2E_ROOT_OWNED, it holds the marker file, and it is a `meroku-e2e-*`
 * directory directly inside the OS temp directory. A root passed in from the
 * environment fails the first check and is never deleted.
 */
export function isOwnedRunRoot(root: string): boolean {
	if (process.env.MEROKU_E2E_ROOT_OWNED !== root) return false;
	if (!existsSync(path.join(root, E2E_ROOT_MARKER))) return false;
	const real = realpathSync(root);
	return (
		path.dirname(real) === realpathSync(tmpdir()) &&
		path.basename(real).startsWith("meroku-e2e-")
	);
}

/**
 * Removes the run's temporary root, and only one the run created. Set
 * MEROKU_E2E_KEEP_ROOT=1 to keep it, for example to inspect the YAML a failing
 * test left behind.
 */
export default function globalTeardown(): void {
	const root = process.env.MEROKU_E2E_ROOT;
	if (!root) return;
	if (process.env.MEROKU_E2E_KEEP_ROOT === "1") {
		console.log(`Kept the e2e project root: ${root}`);
		return;
	}
	if (!isOwnedRunRoot(root)) {
		console.log(`Not deleting ${root}: this run did not create it.`);
		return;
	}
	rmSync(root, { recursive: true, force: true });
}

import { cpSync, mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { test as base, expect } from "@playwright/test";
import yaml from "js-yaml";
import { MerokuApp } from "../pages/meroku-app";
import { fixtureProjectDir, projectDir, screenshotDir } from "./paths";

/** The env YAML meroku serves, read straight from disk. */
export interface EnvYaml {
	/** Absolute path of `<env>.yaml` in the served project directory. */
	path(env?: string): string;
	/** The file's text as it is on disk now. */
	text(env?: string): string;
	/** The file parsed as YAML. */
	read(env?: string): Record<string, unknown>;
}

/** Saves a screenshot of the current page as evidence and attaches it to the report. */
export type Evidence = (name: string) => Promise<string>;

interface HarnessFixtures {
	envYaml: EnvYaml;
	evidence: Evidence;
	app: MerokuApp;
}

export const test = base.extend<HarnessFixtures>({
	// Auto: every test starts from the fixture YAML, whatever the previous test saved.
	envYaml: [
		// biome-ignore lint/correctness/noEmptyPattern: Playwright fixtures take their dependencies as a destructured first argument.
		async ({}, use) => {
			cpSync(fixtureProjectDir, projectDir(), { recursive: true });
			const file = (env = "dev") => path.join(projectDir(), `${env}.yaml`);
			const text = (env?: string) => readFileSync(file(env), "utf8");
			await use({
				path: file,
				text,
				read: (env) => yaml.load(text(env)) as Record<string, unknown>,
			});
		},
		{ auto: true },
	],

	evidence: async ({ page }, use, testInfo) => {
		const dir = screenshotDir();
		mkdirSync(dir, { recursive: true });
		await use(async (name) => {
			const file = path.join(dir, `${name}.png`);
			await page.screenshot({ path: file });
			await testInfo.attach(name, { path: file, contentType: "image/png" });
			return file;
		});
	},

	app: async ({ page }, use) => {
		await use(new MerokuApp(page));
	},
});

export { expect };

// Builds meroku from ../app and serves its API from a throwaway project
// directory holding the synthetic fixture YAML. Started by playwright.config.ts
// as a web server; it is not meant to be run by hand.
//
// The AWS SDK is pointed at empty config and credentials files and the
// instance metadata lookup is off, so nothing the server does can reach a real
// AWS account: every AWS-backed endpoint fails fast instead.

import { spawn, spawnSync } from "node:child_process";
import { cpSync, mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const webDir = path.resolve(
	path.dirname(fileURLToPath(import.meta.url)),
	"../../..",
);
const appDir = path.resolve(webDir, "../app");
const fixtureProjectDir = path.join(webDir, "tests/e2e/fixtures/project");

const root = process.env.MEROKU_E2E_ROOT;
const port = process.env.MEROKU_E2E_API_PORT;
if (!root || !port) {
	console.error(
		"MEROKU_E2E_ROOT and MEROKU_E2E_API_PORT must be set; start the suite with `pnpm test:e2e`",
	);
	process.exit(2);
}

const projectDir = path.join(root, "project");
const binDir = path.join(root, "bin");
const awsDir = path.join(root, "aws");
const binary = path.join(
	binDir,
	process.platform === "win32" ? "meroku.exe" : "meroku",
);

mkdirSync(projectDir, { recursive: true });
mkdirSync(binDir, { recursive: true });
mkdirSync(awsDir, { recursive: true });
cpSync(fixtureProjectDir, projectDir, { recursive: true });
writeFileSync(path.join(awsDir, "config"), "");
writeFileSync(path.join(awsDir, "credentials"), "");

const build = spawnSync("go", ["build", "-o", binary, "."], {
	cwd: appDir,
	stdio: "inherit",
});
if (build.error || build.status !== 0) {
	console.error("go build failed", build.error ?? "");
	process.exit(build.status ?? 1);
}

const env = { ...process.env };
for (const key of [
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_DEFAULT_PROFILE",
	"AWS_ROLE_ARN",
	"AWS_WEB_IDENTITY_TOKEN_FILE",
	"AWS_CONTAINER_CREDENTIALS_FULL_URI",
	"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
]) {
	delete env[key];
}
Object.assign(env, {
	AWS_CONFIG_FILE: path.join(awsDir, "config"),
	AWS_SHARED_CREDENTIALS_FILE: path.join(awsDir, "credentials"),
	AWS_PROFILE: "myapp-e2e",
	AWS_REGION: "us-east-1",
	AWS_EC2_METADATA_DISABLED: "true",
});

const server = spawn(
	binary,
	["--web", "--web-port", port, "--profile", "myapp-e2e", "--theme", "dark"],
	{ cwd: projectDir, env, stdio: "inherit" },
);

for (const signal of ["SIGINT", "SIGTERM"]) {
	process.on(signal, () => server.kill(signal));
}
server.on("exit", (code, signal) => {
	process.exit(code ?? (signal ? 1 : 0));
});

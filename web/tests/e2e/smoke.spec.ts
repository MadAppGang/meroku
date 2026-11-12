import { expect, test } from "./support/harness";

test("Canvas · dev environment · opens the backend service properties", async ({
	app,
	envYaml,
	evidence,
}) => {
	expect(envYaml.read().project).toBe("myapp");

	await app.open("dev");
	await app.openNode("Backend service");

	await expect(app.closePanelButton()).toBeVisible();
	await evidence("smoke-backend-panel");
});

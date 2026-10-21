import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

// Usage from repository root: node web/scripts/test-property-panel-v3-browser.mjs
// Requires Storybook running on port 6014, web dependencies, and Playwright Chromium.
// All 16 cases run with defaults. V3_FRAME_URL may select another running Storybook.
// Fresh Chromium by default; V3_FRESH_BROWSER=0 opts into CDP.
// Optional: V3_WEB_PATH, V3_FRAME_URL, V3_CDP_URL, V3_HOOK_PROBE_MODULE.
// Default hook probe: /src/components/properties-v3/__tests__/publicHookProbe.tsx.
const require = createRequire(import.meta.url);
const webPath =
	process.env.V3_WEB_PATH ?? fileURLToPath(new URL("../", import.meta.url));
const { chromium } = require(
	require.resolve("playwright", { paths: [webPath] }),
);
const harnessPath = "/src/components/properties-v3/__tests__/uiHarness.tsx";
const frameURL =
	process.env.V3_FRAME_URL ??
	"http://127.0.0.1:6014/iframe.html?id=panels-v3-group--default&viewMode=story";
const cdpURL = process.env.V3_CDP_URL ?? "http://127.0.0.1:56909";
let browser;
let launched = false;
if (process.env.V3_FRESH_BROWSER === "0") {
	try {
		browser = await chromium.connectOverCDP(cdpURL, { timeout: 5000 });
	} catch (error) {
		console.warn(
			`CDP unavailable; launching isolated Chromium (${error.message.split("\n")[0]})`,
		);
	}
}
if (!browser) {
	browser = await chromium.launch({ headless: true });
	launched = true;
}
const context = await browser.newContext({
	timezoneId: "UTC",
	locale: "en-US",
});
const page = await context.newPage();
page.setDefaultTimeout(10000);
// Prevent accidental AWS calls. The harness itself provides only deferred adapters.
await context.route(
	/https?:\/\/[^/]*(?:amazonaws\.com|aws\.amazon\.com)(?:\/|$)/i,
	(route) => route.abort("blockedbyclient"),
);
await page.goto(frameURL, { waitUntil: "domcontentloaded" });
await page.evaluate(async (path) => {
	window.__v3Harness = await import(path);
}, harnessPath);

const panel = () => page.getByTestId("v3-black-box-panel");
const image = () =>
	panel().getByRole("textbox", { name: "Synthetic image", exact: true });
const save = () =>
	panel().getByRole("button", { name: /^Save configuration(?:\b|$)/i });
const discard = () =>
	panel().getByRole("button", { name: "Discard all changes", exact: true });
const records = () => page.evaluate(() => window.__v3Harness.records());
async function invoke(method, ...args) {
	return page.evaluate(
		async ({ method, args }) => {
			const result = window.__v3Harness[method](...args);
			// Flush promise continuations and let React commit; no wall-clock sleeps.
			await result;
			await new Promise((resolve) => requestAnimationFrame(resolve));
			return result;
		},
		{ method, args },
	);
}
async function waitInput(locator, expected) {
	await locator.waitFor({ state: "visible" });
	// Locator-based polling preserves accessible-label selection without DOM assumptions.
	await poll(
		async () => (await locator.inputValue()) === expected,
		`input value ${JSON.stringify(expected)}`,
	);
}
async function poll(predicate, description) {
	const deadline = Date.now() + 10000;
	while (!(await predicate())) {
		if (Date.now() > deadline)
			throw new Error(`Timed out waiting for ${description}`);
		await page.evaluate(
			() => new Promise((resolve) => requestAnimationFrame(resolve)),
		);
	}
}
async function tab(name) {
	const label = new RegExp(`^${name}$`, "i");
	await panel()
		.getByRole("tab", { name: label })
		.or(panel().getByRole("button", { name: label, exact: true }))
		.click();
}
async function confirmDialog() {
	const dialog = page.getByRole("dialog").or(page.getByRole("alertdialog"));
	await dialog.waitFor({ state: "visible" });
	const candidates = dialog.getByRole("button", {
		name: /confirm|save configuration|save changes|save anyway/i,
	});
	assert.equal(
		await candidates.count(),
		1,
		"Destructive dialog needs one identifiable confirmation action",
	);
	await candidates.click();
}
async function failSave(index = 0) {
	await invoke("settleSave", index, {
		ok: false,
		message: "Synthetic persistence failure",
	});
	await panel()
		.getByText("Synthetic persistence failure", { exact: true })
		.waitFor({ state: "visible" });
}

const results = [];
async function test(name, run) {
	try {
		await invoke("mount");
		await panel().waitFor({ state: "visible" });
		await run();
		results.push({ name, status: "PASS" });
		console.log(`PASS ${name}`);
	} catch (error) {
		results.push({ name, status: "FAIL", error: error.stack ?? String(error) });
		console.error(`FAIL ${name}\n${error.stack ?? error}`);
	}
}

try {
	await test("failed save and Dismiss preserve edits and original baseline", async () => {
		await image().fill("synthetic:unsaved");
		assert.equal((await records()).saves.length, 0, "Typing must not save");
		await save().click();
		await poll(
			async () => (await records()).saves.length === 1,
			"save adapter request",
		);
		await failSave();
		await panel()
			.getByRole("button", { name: /dismiss/i })
			.click();
		await waitInput(image(), "synthetic:unsaved");
		assert.equal(await save().isEnabled(), true);
		assert.equal((await records()).saves.length, 1, "Dismiss must not persist");
		await discard().click();
		await waitInput(image(), "synthetic:old");
	});

	await test("multi-character map key/value edits retain focus and record shape", async () => {
		// Values belong to the independent fixture, not production DOM classes.
		const keyInput = panel().locator('input[value="SYNTHETIC_KEY"]');
		await keyInput.waitFor({ state: "visible" });
		const keyHandle = await keyInput.elementHandle();
		await keyInput.focus();
		await keyInput.press("End");
		await page.keyboard.type("_MORE");
		assert.equal(
			await keyHandle.evaluate(
				(element) => element.isConnected && document.activeElement === element,
			),
			true,
			"Key input must not unmount or lose focus after each character",
		);
		assert.equal(await keyHandle.inputValue(), "SYNTHETIC_KEY_MORE");
		const valueInput = panel().locator('input[value="fake-value"]');
		await valueInput.waitFor({ state: "visible" });
		const valueHandle = await valueInput.elementHandle();
		await valueInput.focus();
		await valueInput.press("End");
		await page.keyboard.type("-MORE");
		assert.equal(
			await valueHandle.evaluate(
				(element) => element.isConnected && document.activeElement === element,
			),
			true,
			"Value input must retain focus while typing",
		);
		assert.equal(await valueHandle.inputValue(), "fake-value-MORE");
		assert.equal((await records()).saves.length, 0);
		await save().click();
		await poll(
			async () => (await records()).saves.length === 1,
			"environment save",
		);
		const output = await records();
		assert.deepEqual(
			output.submittedBaselines[0].environment,
			{ SYNTHETIC_KEY_MORE: "fake-value-MORE" },
			"Reviewed operations must reconstruct a record, not positional rows or character keys",
		);
		assert.ok(
			output.saves[0].changes.every(
				(change) => change.impact === "rolling-restart",
			),
		);
	});

	await test("same-node prop refresh updates telemetry and timestamp without discarding config", async () => {
		await image().fill("synthetic:dirty-during-refresh");
		await tab("Operations");
		const before = await panel().innerText();
		assert.match(before, /12/);
		await invoke("updateTelemetry");
		await poll(
			async () => /73/.test(await panel().innerText()),
			"new CPU reading",
		);
		// Accept visible timestamp text or public semantic timestamp attributes;
		// do not require one implementation-specific DOM layout/date formatter.
		const after = await panel().evaluate(
			(element) =>
				element.innerText +
				" " +
				[...element.querySelectorAll("[datetime],[title],[aria-label]")]
					.map((item) =>
						[
							item.getAttribute("datetime"),
							item.getAttribute("title"),
							item.getAttribute("aria-label"),
						].join(" "),
					)
					.join(" "),
		);
		assert.match(
			after,
			/(?:04:05|4:05|2026-09-09T04:05)/,
			"Telemetry timestamp must refresh to the new supplied instant (UTC)",
		);
		assert.doesNotMatch(after, /(?:01:02:03|1:02:03|2026-09-09T01:02:03)/);
		await tab("Configuration");
		await waitInput(image(), "synthetic:dirty-during-refresh");
		await save().click();
		await poll(
			async () => (await records()).saves.length === 1,
			"save after telemetry rerender",
		);
		const changes = (await records()).saves[0].changes;
		assert.equal(changes.length, 1);
		assert.equal(changes[0].fieldId, "image");
		assert.equal(changes[0].before, "synthetic:old");
		assert.equal(changes[0].after, "synthetic:dirty-during-refresh");
	});

	for (const order of [
		[0, 1],
		[1, 0],
	]) {
		await test(`overlapping actions both settle in completion order ${order.join(",")}`, async () => {
			await tab("Operations");
			const one = panel().getByRole("button", {
				name: /Synthetic action one/i,
			});
			const two = panel().getByRole("button", {
				name: /Synthetic action two/i,
			});
			await one.click();
			await two.click();
			await poll(
				async () => (await records()).actions.length === 2,
				"two overlapping action requests",
			);
			for (const index of order) await invoke("settleAction", index);
			await poll(
				async () => (await one.isEnabled()) && (await two.isEnabled()),
				"both actions leave pending state",
			);
			await one.click();
			await two.click();
			await poll(
				async () => (await records()).actions.length === 4,
				"both actions can be invoked again",
			);
			assert.equal(
				(await records()).saves.length,
				0,
				"Actions never save configuration",
			);
		});
	}

	await test("deployed=false initially is not reported as no changes to deploy", async () => {
		const text = await panel().innerText();
		assert.doesNotMatch(text, /no changes to deploy/i);
	});

	await test("destructive cancel preserves draft; confirmation sends exact review; failure preserves it", async () => {
		const database = panel().getByRole("textbox", {
			name: "Synthetic database name",
			exact: true,
		});
		await database.fill("replacement_db");
		await image().fill("synthetic:reviewed");
		await save().click();
		let dialog = page.getByRole("dialog").or(page.getByRole("alertdialog"));
		await dialog.waitFor({ state: "visible" });
		const review = await dialog.innerText();
		assert.match(review, /Synthetic PostgreSQL/);
		assert.match(review, /postgres\.dbname|\bdbname\b/);
		assert.match(review, /deploy/i);
		assert.equal((await records()).saves.length, 0);
		await dialog.getByRole("button", { name: /^cancel$/i }).click();
		await dialog.waitFor({ state: "hidden" });
		await waitInput(database, "replacement_db");
		await waitInput(image(), "synthetic:reviewed");
		assert.equal((await records()).saves.length, 0);
		await save().click();
		await confirmDialog();
		await poll(
			async () => (await records()).saves.length === 1,
			"confirmed payload",
		);
		const request = (await records()).saves[0];
		assert.equal(request.nodeInstanceId, "synthetic-node-A");
		assert.equal(request.channel, "configuration");
		assert.deepEqual(
			request.changes
				.map(({ fieldId, before, after, impact }) => ({
					fieldId,
					before,
					after,
					impact,
				}))
				.sort((a, b) => a.fieldId.localeCompare(b.fieldId)),
			[
				{
					fieldId: "dbname",
					before: "synthetic_db",
					after: "replacement_db",
					impact: "recreates-data-loss",
				},
				{
					fieldId: "image",
					before: "synthetic:old",
					after: "synthetic:reviewed",
					impact: "applies-on-deploy",
				},
			],
		);
		await failSave();
		await waitInput(database, "replacement_db");
		await waitInput(image(), "synthetic:reviewed");
		await panel()
			.getByRole("button", { name: /dismiss/i })
			.click();
		await save().click();
		dialog = page.getByRole("dialog").or(page.getByRole("alertdialog"));
		await dialog.waitFor({ state: "visible" });
		assert.equal(
			(await records()).saves.length,
			1,
			"Failure must not retain reusable destructive approval",
		);
		await dialog.getByRole("button", { name: /^cancel$/i }).click();
		await discard().click();
		await waitInput(database, "synthetic_db");
		await waitInput(image(), "synthetic:old");
	});

	for (const staleResult of [
		{ ok: true },
		{ ok: false, message: "Stale node A failure" },
	]) {
		await test(`node switch ignores stale ${staleResult.ok ? "success" : "failure"}`, async () => {
			await image().fill("synthetic:node-A-pending");
			await save().click();
			await poll(
				async () => (await records()).saves.length === 1,
				"node A pending request",
			);
			await invoke("switchNode");
			await waitInput(image(), "synthetic:node-B");
			await image().fill("synthetic:node-B-dirty");
			await invoke("settleSave", 0, staleResult);
			await waitInput(image(), "synthetic:node-B-dirty");
			assert.doesNotMatch(await panel().innerText(), /Stale node A failure/);
			await poll(
				async () => await save().isEnabled(),
				"node B dirty save is enabled",
			);
			await save().click();
			await poll(
				async () => (await records()).saves.length === 2,
				"node B request",
			);
			const request = (await records()).saves[1];
			assert.equal(request.nodeInstanceId, "synthetic-node-B");
			assert.equal(request.changes.length, 1);
			assert.equal(request.changes[0].before, "synthetic:node-B");
			assert.equal(request.changes[0].after, "synthetic:node-B-dirty");
		});
	}

	await test("two public setValue calls in one event retain both fields", async () => {
		await page.evaluate(async (path) => {
			const { default: Probe } = await import(path);
			window.__v3Harness.mountPublicHookProbe(Probe);
		}, process.env.V3_HOOK_PROBE_MODULE ??
			"/src/components/properties-v3/__tests__/publicHookProbe.tsx");
		await page
			.getByRole("button", { name: "Batch public setValue", exact: true })
			.click();
		await poll(
			async () =>
				(await page
					.getByRole("textbox", { name: "Batched first", exact: true })
					.inputValue()) === "first-updated" &&
				(await page
					.getByRole("textbox", { name: "Batched second", exact: true })
					.inputValue()) === "second-updated",
			"both batched public updates",
		);
	});
	await test("configuration and live saves settling together retain both baselines", async () => {
		await page.evaluate(async () => {
			const { ConcurrentSaveProbe } = await import(
				"/src/components/properties-v3/__tests__/publicHookProbe.tsx"
			);
			window.__v3Harness.mountPublicHookProbe(ConcurrentSaveProbe);
		});
		const state = async () =>
			JSON.parse(await page.getByTestId("concurrent-save-state").innerText());
		await page
			.getByRole("button", { name: "Stage both channels", exact: true })
			.click();
		await poll(
			async () => (await state()).dirtyChannels.length === 2,
			"both channels dirty",
		);
		await page
			.getByRole("button", { name: "Submit both channels", exact: true })
			.click();
		await poll(async () => {
			const current = await state();
			return (
				current.status.configuration === "submitting" &&
				current.status.live === "submitting"
			);
		}, "both requests in flight together");
		await page
			.getByRole("button", { name: "Resolve both saves together", exact: true })
			.click();
		await poll(async () => {
			const current = await state();
			return (
				current.status.configuration === "idle" &&
				current.status.live === "idle"
			);
		}, "both requests settled");
		const current = await state();
		assert.equal(current.baseline.first, "first-updated");
		assert.equal(current.baseline.second, "second-updated");
		assert.equal(current.draft.first, "first-updated");
		assert.equal(current.draft.second, "second-updated");
		assert.deepEqual(current.dirtyChannels, []);
	});

	await test("chained visibility resets dirty dependent using baseline-only controller values", async () => {
		await page.evaluate(async () => {
			const { ChainedVisibilityProbe } = await import(
				"/src/components/properties-v3/__tests__/publicHookProbe.tsx"
			);
			window.__v3Harness.mountPublicHookProbe(ChainedVisibilityProbe);
		});
		const state = async () =>
			JSON.parse(
				await page.getByTestId("chained-visibility-state").innerText(),
			);
		assert.deepEqual((await state()).values, {
			gate: true,
			controller: false,
			dependent: "baseline-dependent",
		});
		await page
			.getByRole("button", { name: "Enable dependent", exact: true })
			.click();
		await poll(
			async () => (await state()).values.controller === true,
			"controller enabled through baseline gate",
		);
		await page
			.getByRole("button", { name: "Edit dependent", exact: true })
			.click();
		await poll(
			async () => (await state()).values.dependent === "dirty-dependent",
			"dependent dirty",
		);
		await page
			.getByRole("button", { name: "Hide controller chain", exact: true })
			.click();
		await poll(async () => {
			const current = await state();
			return (
				current.values.controller === false &&
				current.values.dependent === "baseline-dependent"
			);
		}, "visibility reset reaches dependent after controller baseline restoration");
		const current = await state();
		assert.equal(current.values.gate, false);
		assert.deepEqual(
			current.changes.map((change) => change.fieldId),
			["gate"],
		);
		assert.ok(
			current.notice?.fieldLabels.includes("Visibility dependent"),
			"Notice must name the discarded dependent edit",
		);
	});
	await test("typed string-array textarea roundtrips an array, not serialized text", async () => {
		const input = panel().getByRole("textbox", {
			name: "Synthetic string list",
			exact: true,
		});
		const original = ["synthetic-first", "synthetic-second"];
		const edited = ["synthetic-updated", "synthetic-added", "synthetic-third"];
		const current = await input.inputValue();
		// Use the editor's PUBLIC displayed notation, not an assumed formatter.
		// The assertion is always the same typed array at the save boundary.
		let rendered;
		try {
			const parsed = JSON.parse(current);
			if (Array.isArray(parsed)) {
				assert.deepEqual(parsed, original);
				rendered = JSON.stringify(edited);
			}
		} catch (error) {
			if (!(error instanceof SyntaxError)) throw error;
		}
		if (rendered === undefined && current === original.join("\n"))
			rendered = edited.join("\n");
		if (rendered === undefined && current === original.join(","))
			rendered = edited.join(",");
		assert.notEqual(
			rendered,
			undefined,
			"Editor must expose a recognizable representation of the initial string array",
		);
		await input.fill(rendered);
		assert.equal((await records()).saves.length, 0);
		await save().click();
		await poll(
			async () => (await records()).saves.length === 1,
			"typed array save",
		);
		const output = await records();
		assert.equal(output.saves[0].changes.length, 1);
		assert.equal(output.saves[0].changes[0].fieldId, "string-list");
		assert.deepEqual(output.saves[0].changes[0].after, edited);
		assert.deepEqual(output.submittedBaselines[0]["string-list"], edited);
	});

	await test("first entry in empty scalar environment map has editable value and preserves scalar shape", async () => {
		await invoke("mount", { emptyEnvironment: true });
		await panel()
			.getByRole("button", {
				name: "Add synthetic environment row",
				exact: true,
			})
			.click();
		const emptyInputs = panel().locator('input[value=""]');
		await poll(
			async () => (await emptyInputs.count()) >= 2,
			"new entry key and value editors",
		);
		await emptyInputs.first().fill("SYNTHETIC_FIRST");
		await panel().locator('input[value=""]').last().fill("fake-first-value");
		await save().click();
		await poll(
			async () => (await records()).saves.length === 1,
			"new scalar map entry save",
		);
		const output = await records();
		assert.deepEqual(output.submittedBaselines[0].environment, {
			SYNTHETIC_FIRST: "fake-first-value",
		});
		assert.ok(
			output.saves[0].changes.every(
				(change) => change.impact === "rolling-restart",
			),
		);
	});
	await test("all 18 public Panels-v3 CSF variants have valid unchanged baselines", async () => {
		const catalog = await page.evaluate(async () => {
			const response = await fetch("/index.json");
			if (!response.ok)
				throw new Error(`Storybook public index returned ${response.status}`);
			const index = await response.json();
			const entries = Object.values(index.entries ?? {}).filter(
				(entry) =>
					entry.type === "story" && entry.title.startsWith("Panels-v3/"),
			);
			const { computeDiff } = await import(
				"/src/components/properties-v3/diffEngine.ts"
			);
			const results = [];
			for (const entry of entries) {
				if (!entry.importPath || !entry.exportName) {
					throw new Error(
						`Public story ${entry.id} lacks importPath/exportName`,
					);
				}
				// Consume CSF exports as PUBLIC DATA. Do not fetch or inspect source text.
				const modulePath = entry.importPath.startsWith("./")
					? `/${entry.importPath.slice(2)}`
					: entry.importPath;
				const module = await import(modulePath);
				const story = module[entry.exportName];
				if (!story)
					throw new Error(
						`Public CSF export ${entry.exportName} missing for ${entry.id}`,
					);
				const args = { ...(module.default?.args ?? {}), ...(story.args ?? {}) };
				const { definition, instance } = args;
				if (!definition || !instance || !instance.baseline) {
					throw new Error(
						`Public story ${entry.id} must expose definition and instance.baseline args`,
					);
				}
				const result = computeDiff(
					definition,
					instance.baseline,
					instance.baseline,
					instance,
					{ resource: instance.resource },
				);
				results.push({
					id: entry.id,
					title: entry.title,
					exportName: entry.exportName,
					validationErrors: result.validationErrors,
				});
			}
			return results;
		});
		assert.equal(
			catalog.length,
			18,
			"Validate every expected public CSF variant; never pass vacuously on an empty index",
		);
		const invalid = catalog.filter(
			(entry) => entry.validationErrors.length > 0,
		);
		assert.deepEqual(
			invalid,
			[],
			`Catalog baselines must validate without edits: ${JSON.stringify(invalid, null, 2)}`,
		);
	});
	await test("actual S3 CSF allowed_methods JSON edit saves a typed array cell", async () => {
		const fixture = await page.evaluate(async () => {
			const response = await fetch("/index.json");
			if (!response.ok)
				throw new Error(`Storybook public index returned ${response.status}`);
			const index = await response.json();
			const entries = Object.values(index.entries ?? {}).filter(
				(entry) =>
					entry.type === "story" && /^Panels-v3\/S3$/i.test(entry.title),
			);
			const entry =
				entries.find(
					(item) => item.exportName === "Default" || item.name === "Default",
				) ?? entries[0];
			if (!entry)
				throw new Error(
					"Public S3 CSF fixture is required for catalog integration regression",
				);
			const path = entry.importPath.startsWith("./")
				? `/${entry.importPath.slice(2)}`
				: entry.importPath;
			const module = await import(path);
			const args = {
				...(module.default?.args ?? {}),
				...(module[entry.exportName]?.args ?? {}),
			};
			const { definition, instance } = args;
			if (!definition || !instance)
				throw new Error(
					"S3 CSF must expose public definition and instance args",
				);
			const corsField = (definition.fields ?? []).find((field) =>
				field.collection?.columns?.some(
					(column) => column.key === "allowed_methods",
				),
			);
			if (!corsField)
				throw new Error(
					"Public S3 schema must declare the allowed_methods collection column",
				);
			const column = corsField.collection.columns.find(
				(column) => column.key === "allowed_methods",
			);
			if (column.valueType !== "string-array")
				throw new Error(
					"S3 allowed_methods public column must be string-array",
				);
			const { resolveFieldValue } = await import(
				"/src/components/properties-v3/helpers.ts"
			);
			const rows = resolveFieldValue(instance.baseline, corsField);
			if (!Array.isArray(rows) || !rows.length)
				throw new Error(
					"S3 public fixture needs a representative existing CORS row",
				);
			window.__v3ActualCorsField = corsField;
			window.__v3Harness.mountPublicPanel(definition, instance);
			return {
				rows,
				fieldId: corsField.id,
				columnLabel: column.label ?? column.key,
				nodeInstanceId: instance.nodeInstanceId,
			};
		});
		const escapedLabel = fixture.columnLabel.replace(
			/[.*+?^${}()|[\]\\]/g,
			"\\$&",
		);
		const methods = panel()
			.getByRole("textbox", {
				name: new RegExp(`${escapedLabel}|allowed[\\s_-]*methods`, "i"),
			})
			.first();
		await methods.waitFor({ state: "visible" });
		// Normally GET/POST; choose another valid array if that is already the
		// fixture baseline so this remains a genuine edit, not a vacuous no-op.
		const desired =
			JSON.stringify(fixture.rows[0].allowed_methods) ===
			JSON.stringify(["GET", "POST"])
				? ["GET", "POST", "HEAD"]
				: ["GET", "POST"];
		await methods.fill(JSON.stringify(desired));
		assert.equal(
			(await records()).saves.length,
			0,
			"Typed CORS edits stage locally",
		);
		await save().click();
		await poll(
			async () => (await records()).saves.length === 1,
			"actual S3 CORS save request",
		);
		const output = await page.evaluate(async () => {
			const { resolveFieldValue } = await import(
				"/src/components/properties-v3/helpers.ts"
			);
			const captured = window.__v3Harness.records();
			return {
				request: captured.saves[0],
				rows: resolveFieldValue(
					captured.submittedBaselines[0],
					window.__v3ActualCorsField,
				),
			};
		});
		assert.equal(output.request.nodeInstanceId, fixture.nodeInstanceId);
		assert.equal(output.request.channel, "configuration");
		assert.equal(output.request.changes.length, 1);
		assert.equal(output.request.changes[0].fieldId, fixture.fieldId);
		assert.deepEqual(
			output.rows,
			fixture.rows.map((row, index) =>
				index === 0 ? { ...row, allowed_methods: desired } : row,
			),
			"Actual catalog typed cell must remain an array, with other CORS cells/rows preserved",
		);
		assert.ok(
			Array.isArray(output.rows[0].allowed_methods),
			"Never save serialized JSON text as the allowed_methods cell",
		);
	});
} finally {
	await invoke("unmount").catch(() => {});
	await context.close();
	if (launched) await browser.close();
}
const exitCode = results.some((result) => result.status === "FAIL") ? 1 : 0;
// Exit this client after flushing results; never send a browser-close command
// to the parent's CDP browser. Its only owned context was closed above.
process.stdout.write(`${JSON.stringify({ results }, null, 2)}\n`, () =>
	process.exit(exitCode),
);

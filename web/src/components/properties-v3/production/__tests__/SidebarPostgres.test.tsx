/**
 * The shipped Sidebar, PostgreSQL node, legacy vs ?panels=v3.
 *
 * Legacy persists every keystroke of a replacement (TF-REPLACE) field; V3
 * must persist nothing until Save, and nothing destructive until the
 * confirmation is accepted. Runs in real Chromium (vitest browser mode).
 */

import { act, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";
import type { ComponentNode } from "../../../../types";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { Sidebar } from "../../../Sidebar";
import { PROPERTY_PANELS_STORAGE_KEY } from "../panelFlag";

// Synthetic config: placeholder account, example.invalid-style names only.
const baseConfig = {
	project: "myapp",
	env: "dev",
	is_prod: false,
	region: "us-east-1",
	account_id: "000000000000",
	state_bucket: "myapp-terraform-state-example",
	state_file: "state.tfstate",
	postgres: {
		enabled: true,
		dbname: "myapp",
		username: "postgres",
		aurora: false,
		instance_class: "db.t4g.micro",
		allocated_storage: 20,
		storage_encrypted: true,
	},
} as YamlInfrastructureConfig;

const postgresNode = {
	id: "postgres",
	type: "postgres",
	name: "PostgreSQL",
} as unknown as ComponentNode;

let root: Root | null = null;
let host: HTMLElement | null = null;
let updates: Partial<YamlInfrastructureConfig>[] = [];

/** Mount the Sidebar with a parent that applies updates like App does. */
async function mountSidebar() {
	updates = [];
	host = document.createElement("div");
	document.body.appendChild(host);
	const mounted = createRoot(host);
	root = mounted;

	let config = baseConfig;
	const render = () => mounted.render(<Sidebar {...props()} />);
	const props = (): ComponentProps<typeof Sidebar> => ({
		selectedNode: postgresNode,
		isOpen: true,
		onClose: () => {},
		config,
		onConfigChange: (update) => {
			updates.push(structuredClone(update));
			config = {
				...config,
				...update,
				postgres: { ...config.postgres, ...update.postgres },
			} as YamlInfrastructureConfig;
			render();
		},
	});
	await act(async () => render());
}

function setFlag(value: "v3" | "legacy") {
	window.localStorage.setItem(PROPERTY_PANELS_STORAGE_KEY, value);
}

beforeEach(() => {
	window.localStorage.removeItem(PROPERTY_PANELS_STORAGE_KEY);
});

afterEach(() => {
	act(() => root?.unmount());
	host?.remove();
	root = null;
	host = null;
});

describe("legacy PostgreSQL panel (default)", () => {
	it("persists every keystroke of the TF-REPLACE database name", async () => {
		setFlag("legacy");
		await mountSidebar();
		const dbname = page.getByLabelText("Database Name", { exact: true });
		await userEvent.fill(dbname, "orders");
		// fill() is one input event; type three more characters one at a time.
		await userEvent.type(dbname, "_v2");
		expect(updates.length).toBeGreaterThanOrEqual(4);
		expect(updates.at(-1)?.postgres?.dbname).toBe("orders_v2");
	});
});

describe("properties-v3 PostgreSQL panel (?panels=v3)", () => {
	async function openConfiguration() {
		setFlag("v3");
		await mountSidebar();
		await page
			.getByRole("navigation", { name: "Property views" })
			.getByRole("button", { name: "Configuration", exact: true })
			.click();
		return page.getByRole("textbox", { name: "Database name", exact: true });
	}

	it("persists nothing while typing a replacement field", async () => {
		const dbname = await openConfiguration();
		await userEvent.fill(dbname, "orders");
		await userEvent.type(dbname, "_v2");
		expect(updates).toEqual([]);
	});

	it("asks for confirmation naming the resource, then saves once", async () => {
		const dbname = await openConfiguration();
		await userEvent.fill(dbname, "orders");
		await page.getByRole("button", { name: /^Save configuration/ }).click();

		const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
		await expect.element(dialog).toBeVisible();
		await expect
			.element(dialog)
			.toHaveTextContent(/myapp-postgres-dev \(RDS instance\)/);
		await expect.element(dialog).toHaveTextContent(/Database name/);
		await expect.element(dialog).not.toHaveTextContent(/myapp_postgres_dev/);
		expect(updates).toEqual([]);

		await page
			.getByRole("button", { name: "Confirm & Save Configuration" })
			.click();
		await expect.poll(() => updates.length).toBe(1);
		expect(updates[0]).toEqual({
			postgres: { ...baseConfig.postgres, dbname: "orders" },
		});
	});

	it("cancelling the confirmation keeps the draft and writes nothing", async () => {
		const dbname = await openConfiguration();
		await userEvent.fill(dbname, "orders");
		await page.getByRole("button", { name: /^Save configuration/ }).click();
		await page.getByRole("button", { name: "Cancel", exact: true }).click();
		expect(updates).toEqual([]);
		await expect.element(dbname).toHaveValue("orders");
	});

	it("saves a non-replacement change without a confirmation", async () => {
		await openConfiguration();
		const storage = page.getByRole("spinbutton", {
			name: "Allocated storage (GB)",
			exact: true,
		});
		await userEvent.fill(storage, "50");
		expect(updates).toEqual([]);
		await page.getByRole("button", { name: /^Save configuration/ }).click();
		await expect.poll(() => updates.length).toBe(1);
		expect(updates[0]?.postgres?.allocated_storage).toBe(50);
	});
});

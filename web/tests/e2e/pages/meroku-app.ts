import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";

/** Actions and readable state of the meroku web app: environment picker, canvas, properties panel. */
export class MerokuApp {
	constructor(private readonly page: Page) {}

	/** Loads the app and picks an environment in the "Select Environment" dialog. */
	async open(env = "dev"): Promise<void> {
		await this.page.goto("/");
		const dialog = this.page.getByRole("dialog", {
			name: "Select Environment",
		});
		await expect(dialog).toBeVisible();
		await dialog.getByRole("combobox", { name: "Environment" }).click();
		await this.page.getByRole("option", { name: env }).click();
		await dialog.getByRole("button", { name: "Continue" }).click();
		await expect(dialog).toBeHidden();
		await expect(this.canvas()).toBeVisible();
	}

	/** The deployment canvas. `rf__wrapper` is the test id React Flow gives its root. */
	canvas(): Locator {
		return this.page.getByTestId("rf__wrapper");
	}

	/** A node on the canvas, by the name it shows. React Flow gives each node the test id `rf__node-<id>`. */
	node(name: string): Locator {
		return this.canvas()
			.getByTestId(/^rf__node-/)
			.filter({ hasText: name });
	}

	/** Opens a node's properties panel by clicking it on the canvas. */
	async openNode(name: string): Promise<void> {
		await this.node(name).first().click();
		await expect(this.panelTitle(name)).toBeVisible();
	}

	/** The properties panel's title for the node it shows. */
	panelTitle(name: string): Locator {
		return this.page.getByRole("heading", { level: 2, name });
	}

	/** The button that closes the properties panel. */
	closePanelButton(): Locator {
		return this.page.getByRole("button", { name: "Close properties" });
	}
}

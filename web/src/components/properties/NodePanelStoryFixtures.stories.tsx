import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import { PROPERTY_NODE_CATALOG } from "./nodeCatalog";
import { PropertyPanel } from "./PropertyPanel";
import type { PropertyPanelDefinition } from "./types";

const meta = {
	title: "Nodes/Properties",
	component: PropertyPanel,
	tags: ["!autodocs"],
	parameters: {
		backgrounds: { disable: true },
		layout: "fullscreen",
	},
	decorators: [
		(Story) => (
			<div className="mp-story-stage" data-surface="light" data-theme="light">
				<Story />
			</div>
		),
	],
	argTypes: {
		definition: { control: false },
		surface: { control: false },
		compact: { control: false },
		initialView: { control: "text" },
		openAdvanced: { control: "boolean" },
		saveState: {
			control: "select",
			options: ["clean", "dirty", "saving", "saved", "error"],
		},
	},
} satisfies Meta<typeof PropertyPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

const assertDraftPanel = async (
	canvasElement: HTMLElement,
	definition: PropertyPanelDefinition,
) => {
	const canvas = within(canvasElement);
	const panel = canvas.getByRole("complementary", {
		name: `${definition.displayName} properties`,
	});
	await expect(panel).toBeVisible();
	await expect(
		canvas.getByRole("heading", { name: definition.name }),
	).toBeVisible();
	await expect(panel).toHaveAttribute("data-density", "compact");
	await expect(panel.querySelectorAll(".mp-section")).toHaveLength(0);
	await expect(panel.querySelectorAll('[role="switch"]')).toHaveLength(0);
	await expect(panel.querySelectorAll(".mp-readonly")).toHaveLength(0);
	await expect(panel.scrollWidth).toBeLessThanOrEqual(panel.clientWidth);
	const detailsButton = canvas.queryByRole("button", { name: "Details" });
	if (detailsButton) {
		await userEvent.click(detailsButton);
		await expect(panel.querySelectorAll(".mp-readonly")).toHaveLength(0);
		await userEvent.click(canvas.getByRole("button", { name: "Setup" }));
	}
	return { canvas, panel };
};

const draftStory = (definition: PropertyPanelDefinition): Story => ({
	args: {
		definition,
		surface: "light",
		compact: true,
		saveState: "clean",
	},
	play: async ({ canvasElement }) => {
		await assertDraftPanel(canvasElement, definition);
	},
});

export const ClientApp: Story = {
	...draftStory(PROPERTY_NODE_CATALOG["client-app"]),
	name: "Draft · Client App",
};
export const GitHub: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.github),
	name: "Draft · GitHub",
	play: async ({ canvasElement }) => {
		const { canvas } = await assertDraftPanel(
			canvasElement,
			PROPERTY_NODE_CATALOG.github,
		);
		await expect(canvas.queryByRole("button", { name: /^Edit / })).toBeNull();
		const oidcToggle = canvas.getByRole("checkbox", { name: "GitHub OIDC" });
		await expect(
			oidcToggle.closest(".mp-compact-group__header"),
		).not.toBeNull();

		await userEvent.click(canvas.getByRole("button", { name: "Add subject" }));
		await expect(
			canvas.queryByRole("button", { name: "Cancel adding subject" }),
		).toBeNull();
		await userEvent.type(
			canvas.getByLabelText("New subject"),
			"repo:madappgang/new-service:*",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Save subject" }));
		await expect(
			canvas.getByText("repo:madappgang/new-service:*"),
		).toBeVisible();
		const removeNewSubject = canvas.getByRole("button", {
			name: "Remove repo:madappgang/new-service:*",
		});
		removeNewSubject.focus();
		await expect(removeNewSubject).toBeVisible();
		await userEvent.click(removeNewSubject);

		for (const row of canvasElement.querySelectorAll<HTMLElement>(
			".mp-compact-tag-list__row",
		)) {
			await expect(row.getBoundingClientRect().height).toBeLessThanOrEqual(36);
		}
		for (const value of canvasElement.querySelectorAll<HTMLElement>(
			".mp-compact-tag-list__row code",
		)) {
			await expect(getComputedStyle(value).whiteSpace).toBe("nowrap");
		}
	},
};
export const APIGateway: Story = {
	...draftStory(PROPERTY_NODE_CATALOG["api-gateway"]),
	name: "Draft · API Gateway",
};
export const ApplicationLoadBalancer: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.alb),
	name: "Draft · Application Load Balancer",
};
export const Route53: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.route53),
	name: "Draft · Route 53",
};
export const ECSCluster: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.ecs),
	name: "Draft · ECS Cluster",
};
export const Backend: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.backend),
	name: "Draft · Backend",
	play: async ({ canvasElement }) => {
		const { canvas } = await assertDraftPanel(
			canvasElement,
			PROPERTY_NODE_CATALOG.backend,
		);
		const port = canvas.getByLabelText("Port").getBoundingClientRect();
		const health = canvas.getByLabelText("Health path").getBoundingClientRect();
		const cpu = canvas.getByLabelText("CPU").getBoundingClientRect();
		const memory = canvas.getByLabelText("Memory").getBoundingClientRect();
		await expect(Math.abs(port.left - cpu.left)).toBeLessThan(2);
		await expect(Math.abs(health.left - memory.left)).toBeLessThan(2);
		await expect(Math.abs(port.top - health.top)).toBeLessThan(2);
		await expect(Math.abs(cpu.top - memory.top)).toBeLessThan(2);
		await expect(
			canvas.queryByRole("heading", { name: "Container & Process" }),
		).toBeNull();

		await canvas.getByRole("button", { name: "Details" }).click();
		await expect(
			canvas.getByRole("heading", { name: "Container & Process" }),
		).toBeVisible();
		await expect(canvas.getByLabelText("Command override")).toBeVisible();
		await expect(
			canvasElement.querySelector(".mp-compact-image-row"),
		).not.toBeNull();
		await canvas.getByRole("button", { name: "Setup" }).click();
	},
};
export const ECR: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.ecr),
	name: "Draft · ECR",
	play: async ({ canvasElement }) => {
		const { canvas } = await assertDraftPanel(
			canvasElement,
			PROPERTY_NODE_CATALOG.ecr,
		);
		const local = canvas.getByRole("button", { name: "This account" });
		const crossAccount = canvas.getByRole("button", {
			name: "Cross-account",
		});
		const repository = canvasElement.querySelector(
			".mp-compact-image-row code",
		);
		await expect(local).toHaveAttribute("aria-pressed", "true");
		await expect(repository).toHaveTextContent("circl_backend");
		await expect(canvasElement.querySelector(".mp-readonly")).toBeNull();

		await userEvent.click(crossAccount);
		await expect(crossAccount).toHaveAttribute("aria-pressed", "true");
		await expect(repository).toHaveTextContent(
			"123456789012.dkr.ecr.ap-southeast-2.amazonaws.com/circl_backend",
		);
		await userEvent.click(local);
		local.blur();
	},
};
export const PostgreSQL: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.postgres),
	name: "Draft · PostgreSQL",
	play: async ({ canvasElement }) => {
		const { canvas } = await assertDraftPanel(
			canvasElement,
			PROPERTY_NODE_CATALOG.postgres,
		);
		const deployment = canvas
			.getByLabelText("Deployment")
			.getBoundingClientRect();
		const username = canvas.getByLabelText("Username").getBoundingClientRect();
		const storage = canvas.getByLabelText("Storage").getBoundingClientRect();
		const database = canvas
			.getByLabelText("Database name")
			.getBoundingClientRect();
		const instanceClass = canvas
			.getByLabelText("Instance class")
			.getBoundingClientRect();
		await expect(Math.abs(deployment.left - username.left)).toBeLessThan(2);
		await expect(Math.abs(deployment.left - storage.left)).toBeLessThan(2);
		await expect(Math.abs(database.left - instanceClass.left)).toBeLessThan(2);

		const advancedButton = canvas.getByRole("button", {
			name: "Advanced settings",
		});
		await userEvent.click(advancedButton);
		const encryption = canvas.getByRole("checkbox", {
			name: "Storage encryption",
		});
		const capabilities = encryption.closest(".mp-compact-runtime-row");
		await expect(
			capabilities?.getBoundingClientRect().width ?? 0,
		).toBeGreaterThan(450);
		await expect(
			encryption.closest(".mp-compact-capability")?.getBoundingClientRect()
				.height ?? 100,
		).toBeLessThan(24);
		await userEvent.click(advancedButton);
		advancedButton.blur();
	},
};
export const Aurora: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.aurora),
	name: "Draft · Aurora",
};
export const S3: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.s3),
	name: "Draft · S3",
};
export const EventBridge: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.eventbridge),
	name: "Draft · EventBridge",
};
export const EventTask: Story = {
	...draftStory(PROPERTY_NODE_CATALOG["event-task"]),
	name: "Draft · Event Task",
};
export const ScheduledTask: Story = {
	...draftStory(PROPERTY_NODE_CATALOG["scheduled-task"]),
	name: "Draft · Scheduled Task",
};
export const SNS: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.sns),
	name: "Draft · SNS",
};
export const SQS: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.sqs),
	name: "Draft · SQS",
};
export const SES: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.ses),
	name: "Draft · SES",
};
export const CloudWatch: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.cloudwatch),
	name: "Draft · CloudWatch",
};
export const XRay: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.xray),
	name: "Draft · X-Ray",
};
export const ParameterStore: Story = {
	...draftStory(PROPERTY_NODE_CATALOG["secrets-manager"]),
	name: "Draft · Parameter Store",
};
export const EFS: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.efs),
	name: "Draft · EFS",
};
export const AppSync: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.appsync),
	name: "Draft · AppSync",
};
export const Amplify: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.amplify),
	name: "Draft · Amplify",
};
export const CloudFront: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.cloudfront),
	name: "Draft · CloudFront",
};
export const CustomTerraform: Story = {
	...draftStory(PROPERTY_NODE_CATALOG["custom-terraform"]),
	name: "Draft · Custom Terraform",
};
export const Alarms: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.alarms),
	name: "Draft · Alarms",
};
export const Group: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.group),
	name: "Draft · Group",
};
export const DynamicGroup: Story = {
	...draftStory(PROPERTY_NODE_CATALOG.dynamicGroup),
	name: "Draft · Dynamic Group",
};

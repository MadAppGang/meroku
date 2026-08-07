import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import { PropertiesPanel as ReferencePropertiesPanel } from "../../reference/components/PropertiesPanel";
import "../../reference/reference-properties.css";
import { PROPERTY_NODE_CATALOG } from "./nodeCatalog";
import { PropertyPanel } from "./PropertyPanel";
import {
	ServiceCompactLedger,
	ServiceCompactPairs,
} from "./ServiceCompactAlternatives";
import { ServiceCompactGrouped } from "./ServiceCompactGrouped";

const meta = {
	title: "Properties/Node Types",
	component: PropertyPanel,
	parameters: { layout: "fullscreen" },
	decorators: [
		(Story) => (
			<div className="mp-story-stage">
				<Story />
			</div>
		),
	],
	argTypes: {
		definition: { control: false },
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

const centerY = (element: Element) => {
	const bounds = element.getBoundingClientRect();
	return bounds.top + bounds.height / 2;
};

const story = (nodeType: string): Story => ({
	name: nodeType,
	args: { definition: PROPERTY_NODE_CATALOG[nodeType], saveState: "clean" },
});

export const ClientApp = story("client-app");
export const GitHub = story("github");
export const APIGateway = story("api-gateway");
export const ApplicationLoadBalancer = story("alb");
export const Route53 = story("route53");
export const ECSCluster = story("ecs");
export const BackendService = story("backend");
export const Service: Story = {
	args: {
		definition: PROPERTY_NODE_CATALOG.service,
		saveState: "clean",
	},
	tags: ["!test"],
	parameters: {
		a11y: { test: "todo" },
		controls: { disable: true },
	},
	render: () => (
		<div className="reference-properties-stage">
			<ReferencePropertiesPanel isOpen togglePanel={() => undefined} />
		</div>
	),
};
export const ServicePrimitives = story("service");
export const ServiceCompactGroupedVersion: Story = {
	name: "service · compact grouped",
	args: {
		definition: PROPERTY_NODE_CATALOG.service,
		saveState: "clean",
	},
	render: () => <ServiceCompactGrouped />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const panel = canvas.getByRole("complementary", {
			name: "terminator properties compact grouped",
		});
		const cpu = canvas.getByLabelText("CPU").getBoundingClientRect();
		const memory = canvas.getByLabelText("Memory").getBoundingClientRect();
		const autoscaling = canvas.getByLabelText("Autoscaling grouped");
		const minimum = canvas.getByLabelText("Minimum grouped");
		const maximum = canvas.getByLabelText("Maximum grouped");
		await expect(panel.getBoundingClientRect().width).toBe(540);
		await expect(
			Number.parseFloat(
				getComputedStyle(canvas.getByText("Runtime & Scaling")).fontSize,
			),
		).toBeGreaterThanOrEqual(13);
		await expect(Math.abs(cpu.top - memory.top)).toBeLessThan(2);
		await expect(autoscaling).toBeChecked();
		await expect(minimum).toHaveValue(1);
		await expect(maximum).toHaveValue(5);
		await expect(canvas.queryByLabelText("Desired grouped")).toBeNull();
		await expect(
			Math.abs(centerY(autoscaling) - centerY(minimum)),
		).toBeLessThan(2);
		await expect(Math.abs(centerY(minimum) - centerY(maximum))).toBeLessThan(2);
		await expect(canvas.getByText("Environment")).toBeVisible();
		await expect(canvas.queryByText("Container & Process")).toBeNull();
		await userEvent.click(autoscaling);
		await expect(autoscaling).not.toBeChecked();
		await expect(canvas.getByLabelText("Desired grouped")).toHaveValue(2);
		await expect(canvas.queryByLabelText("Minimum grouped")).toBeNull();
		await expect(canvas.queryByLabelText("Maximum grouped")).toBeNull();
		await userEvent.click(autoscaling);
		await expect(autoscaling).toBeChecked();
		await userEvent.click(canvas.getByRole("button", { name: "Details" }));
		await expect(
			canvas.getByRole("heading", { name: "Container & Process" }),
		).toBeVisible();
		await expect(canvas.getByLabelText("Command override")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Setup" }));
		await userEvent.click(canvas.getByRole("button", { name: "Discard" }));
	},
};
export const ServiceCreatePopupVersion: Story = {
	name: "service · create popup",
	args: {
		definition: PROPERTY_NODE_CATALOG.service,
		saveState: "clean",
	},
	render: () => (
		<div className="mp-create-dialog-stage">
			<ServiceCompactGrouped mode="create" />
		</div>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const panel = canvas.getByRole("complementary", {
			name: "create service compact grouped",
		});
		const container = canvas.getByRole("heading", {
			name: "Container & Process",
		});
		const runtime = canvas.getByRole("heading", {
			name: "Runtime & Scaling",
		});
		const environment = canvas.getByRole("heading", { name: "Environment" });
		await expect(panel.getBoundingClientRect().width).toBe(540);
		await expect(container).toBeVisible();
		await expect(canvas.getByLabelText("Command override")).toBeVisible();
		await expect(container.getBoundingClientRect().top).toBeLessThan(
			runtime.getBoundingClientRect().top,
		);
		await expect(runtime.getBoundingClientRect().top).toBeLessThan(
			environment.getBoundingClientRect().top,
		);
		await expect(canvas.queryByRole("button", { name: "Details" })).toBeNull();
		await expect(canvas.getByText("Ready to create")).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Create service" }),
		).toBeEnabled();
	},
};
export const ServiceCompactPairsVersion: Story = {
	name: "service · compact inline pairs",
	args: {
		definition: PROPERTY_NODE_CATALOG.service,
		saveState: "clean",
	},
	render: () => <ServiceCompactPairs />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const port = canvas.getByLabelText("Port").getBoundingClientRect();
		const health = canvas.getByLabelText("Health").getBoundingClientRect();
		const cpu = canvas.getByLabelText("CPU").getBoundingClientRect();
		const memory = canvas.getByLabelText("Memory").getBoundingClientRect();
		const autoscaling = canvas.getByLabelText("Autoscaling · 70%");
		const minimum = canvas.getByLabelText("Min");
		const maximum = canvas.getByLabelText("Max");
		await expect(Math.abs(port.top - health.top)).toBeLessThan(2);
		await expect(Math.abs(cpu.top - memory.top)).toBeLessThan(2);
		await expect(
			Math.abs(centerY(autoscaling) - centerY(minimum)),
		).toBeLessThan(2);
		await expect(Math.abs(centerY(minimum) - centerY(maximum))).toBeLessThan(2);
		await expect(autoscaling).toBeChecked();
		await expect(minimum).toHaveValue(1);
		await expect(maximum).toHaveValue(5);
		await expect(canvas.queryByLabelText("Desired")).toBeNull();
		await expect(canvas.getByLabelText("SSH")).not.toBeChecked();
		await userEvent.click(autoscaling);
		await expect(autoscaling).not.toBeChecked();
		await expect(canvas.getByLabelText("Desired")).toHaveValue(2);
		await expect(canvas.queryByLabelText("Min")).toBeNull();
		await expect(canvas.queryByLabelText("Max")).toBeNull();
		await userEvent.click(autoscaling);
		await expect(autoscaling).toBeChecked();
		await userEvent.click(canvas.getByRole("button", { name: "Discard" }));
	},
};
export const ServiceCompactLedgerVersion: Story = {
	name: "service · compact ledger",
	args: {
		definition: PROPERTY_NODE_CATALOG.service,
		saveState: "clean",
	},
	render: () => <ServiceCompactLedger />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cpu = canvas.getByLabelText("CPU ledger").getBoundingClientRect();
		const memory = canvas
			.getByLabelText("Memory ledger")
			.getBoundingClientRect();
		const autoscaling = canvas.getByLabelText("Autoscaling 70%");
		const minimum = canvas.getByLabelText("Min ledger");
		const maximum = canvas.getByLabelText("Max ledger");
		await expect(Math.abs(cpu.top - memory.top)).toBeLessThan(2);
		await expect(
			Math.abs(centerY(autoscaling) - centerY(minimum)),
		).toBeLessThan(2);
		await expect(Math.abs(centerY(minimum) - centerY(maximum))).toBeLessThan(2);
		await expect(autoscaling).toBeChecked();
		await expect(minimum).toHaveValue(1);
		await expect(maximum).toHaveValue(5);
		await expect(canvas.queryByLabelText("Desired ledger")).toBeNull();
		await expect(canvas.getByLabelText("SSH")).not.toBeChecked();
		await userEvent.click(autoscaling);
		await expect(autoscaling).not.toBeChecked();
		await expect(canvas.getByLabelText("Desired ledger")).toHaveValue(2);
		await expect(canvas.queryByLabelText("Min ledger")).toBeNull();
		await expect(canvas.queryByLabelText("Max ledger")).toBeNull();
		await userEvent.click(autoscaling);
		await expect(autoscaling).toBeChecked();
		await userEvent.click(canvas.getByRole("button", { name: "Discard" }));
	},
};
export const ECR = story("ecr");
export const PostgreSQL = story("postgres");
export const S3 = story("s3");
export const EventBridge = story("eventbridge");
export const EventTask = story("event-task");
export const ScheduledTask = story("scheduled-task");
export const SNS = story("sns");
export const SQS = story("sqs");
export const SES = story("ses");
export const CloudWatch = story("cloudwatch");
export const XRay = story("xray");
export const ParameterStore = story("secrets-manager");
export const EFS = story("efs");
export const AppSync = story("appsync");
export const Amplify = story("amplify");
export const CloudFront = story("cloudfront");
export const CustomTerraform = story("custom-terraform");
export const AlarmRules = story("alarms");

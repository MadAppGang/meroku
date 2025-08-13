import type { Meta, StoryObj } from "@storybook/react";
import type { ComponentNode } from "../types";
import { Sidebar } from "./Sidebar";

const meta = {
	title: "Components/Sidebar",
	component: Sidebar,
	parameters: {
		layout: "fullscreen",
	},
	tags: ["autodocs"],
} satisfies Meta<typeof Sidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

const frontendNode: ComponentNode = {
	id: "1",
	name: "frontend",
	type: "frontend",
	status: "running",
	url: "frontend-prod.up.railway.app",
	description: "Deployed just now",
	deploymentType: "via GitHub",
};

const backendNode: ComponentNode = {
	id: "2",
	name: "backend",
	type: "backend",
	status: "running",
	description: "Deployed just now",
	deploymentType: "via GitHub",
	replicas: 3,
};

const databaseNode: ComponentNode = {
	id: "3",
	name: "postgres",
	type: "database",
	status: "running",
	description: "Deployed via Docker Image",
	resources: {
		cpu: "2 vCPU",
		memory: "4GB RAM",
	},
};

const redisNode: ComponentNode = {
	id: "4",
	name: "redis",
	type: "cache",
	status: "running",
	description: "Just deployed",
	resources: {
		cpu: "1 vCPU",
		memory: "1GB RAM",
	},
};

const apiNode: ComponentNode = {
	id: "5",
	name: "api gateway",
	type: "api",
	status: "running",
	url: "api-prod.up.railway.app",
	description: "Deployed just now",
};

const analyticsNode: ComponentNode = {
	id: "6",
	name: "ackee analytics",
	type: "analytics",
	status: "running",
	url: "ackee-prod.up.railway.app",
	description: "Deployed via Docker Image",
};

// Basic sidebar states
export const Closed: Story = {
	args: {
		selectedNode: null,
		isOpen: false,
		onClose: () => console.log("Close clicked"),
	},
};

export const OpenWithFrontend: Story = {
	args: {
		selectedNode: frontendNode,
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

export const OpenWithBackend: Story = {
	args: {
		selectedNode: backendNode,
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

export const OpenWithDatabase: Story = {
	args: {
		selectedNode: databaseNode,
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

export const OpenWithCache: Story = {
	args: {
		selectedNode: redisNode,
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

export const OpenWithAPI: Story = {
	args: {
		selectedNode: apiNode,
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

export const OpenWithAnalytics: Story = {
	args: {
		selectedNode: analyticsNode,
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

// Different status states
export const ServiceDeploying: Story = {
	args: {
		selectedNode: {
			...frontendNode,
			status: "deploying",
		},
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

export const ServiceError: Story = {
	args: {
		selectedNode: {
			...backendNode,
			status: "error",
		},
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

export const ServiceStopped: Story = {
	args: {
		selectedNode: {
			...databaseNode,
			status: "stopped",
		},
		isOpen: true,
		onClose: () => console.log("Close clicked"),
	},
};

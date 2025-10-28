import type { LucideIcon } from "lucide-react";
import type { ECRConfig } from "./yamlConfig";

// Event Task related types - matching YAML config
export interface EventTask {
	name: string;
	rule_name: string;
	detail_types: string[];
	sources: string[];
	docker_image?: string;
	container_command?: string[];
	cpu?: number;
	memory?: number;
	environment_variables?: Record<string, string>;
	ecr_config?: ECRConfig;
}

// Scheduled Task related types - matching YAML config
export interface ScheduledTask {
	name: string;
	schedule: string;
	docker_image?: string;
	container_command?: string;
	cpu?: number;
	memory?: number;
	environment_variables?: Record<string, string>;
	ecr_config?: ECRConfig;
}

// Service related types
export interface Service {
	name: string;
	docker_image: string;
	container_port: number;
	host_port?: number;
	cpu: number;
	memory: number;
	desired_count: number;
	health_check_path: string;
	container_command?: string[];
	environment_variables?: Record<string, string>;
	ecr_config?: ECRConfig;
}

// Amplify related types - matching YAML config structure
export interface AmplifyBranch {
	name: string;
	stage?: "PRODUCTION" | "DEVELOPMENT" | "BETA" | "EXPERIMENTAL";
	enable_auto_build?: boolean;
	enable_pull_request_preview?: boolean;
	environment_variables?: Record<string, string>;
	custom_subdomains?: string[];
}

export interface AmplifyApp {
	name: string;
	github_repository: string;
	branches: AmplifyBranch[];
	custom_domain?: string;
	build_spec?: string;
	framework?: string;
}

// Status and UI related types
export interface StatusConfig {
	color: string;
	icon: LucideIcon;
	text: string;
	pulse?: boolean;
}

export interface BuildStatusConfig {
	color: string;
	bgColor: string;
	icon: LucideIcon;
	text: string;
	priority: number;
	pulse?: boolean;
}

export interface AppStatusConfig {
	color: string;
	bgColor: string;
	icon: LucideIcon;
	text: string;
	priority: number;
}

// Generic update types for components
export type UpdateHandler<T> = (field: string, value: T) => void;
export type BranchUpdateHandler = (
	index: number,
	updates: Partial<AmplifyBranch>,
) => void;

// Node properties type for node state mapping
export interface NodeProperties {
	[key: string]:
		| string
		| number
		| boolean
		| string[]
		| number[]
		| Record<string, string>
		| Record<string, any>[]
		| any;
}

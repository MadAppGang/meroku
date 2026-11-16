import type { ConfigWarning } from "../types/configWarnings";

/**
 * Synthetic configuration warnings shared by the warning stories. Names and
 * values are made up; the shapes follow `GET /api/config-warnings`.
 */

const DOCS = "https://github.com/MadAppGang/meroku/blob/main/ai_docs";

export const serviceNoHealthCheck: ConfigWarning = {
	code: "service_no_health_check",
	item: { kind: "service", name: "magento-bridge" },
	title:
		"service 'magento-bridge' is network_access: internal and has no health_check_command",
	detail:
		"It is not behind the ALB, so nothing health-checks it: ECS replaces a task only\nwhen its process exits. Add a health_check_command that uses a tool the image\nreally has. Slim and distroless images usually have no wget or curl; probe with\nthe runtime instead, or copy the Dockerfile's HEALTHCHECK.",
	docs: `${DOCS}/SERVICE_NETWORK_ACCESS.md#check-that-the-tool-is-in-the-image`,
};

export const servicePolicyWildcard: ConfigWarning = {
	code: "policy_wildcard_action",
	item: { kind: "service", name: "magento-bridge" },
	title: "service 'magento-bridge' grants s3:* in its policy",
	detail:
		"A wildcard action allows every current and future S3 action. List the actions the service calls instead.",
};

export const backendExternalImage: ConfigWarning = {
	code: "backend_external_image_not_pushable",
	item: { kind: "backend", name: "" },
	title: "the backend uses an external image the CI pipeline cannot push",
	detail:
		"workload.backend_image points at a registry meroku does not manage, so the generated GitHub workflow cannot deploy a new build. Push to the project's ECR repository, or deploy the image yourself.",
	docs: `${DOCS}/BACKEND_IMAGE.md`,
};

export const eventTaskProducesAny: ConfigWarning = {
	code: "event_produces_any",
	item: { kind: "event_task", name: "order-sync" },
	title: "event task 'order-sync' may publish any event",
	detail:
		"produces.any: true lets the task put every detail type on the bus. Name the detail types it sends.",
};

export const vpcCidrOverlap: ConfigWarning = {
	code: "vpc_cidr_overlap",
	item: { kind: "env", name: "" },
	title: "vpc_cidr 10.0.0.0/16 overlaps staging",
	detail:
		"Two environments with overlapping ranges can never be peered or share a Transit Gateway.\nPick a free range with `meroku vpc-cidr suggest dev`.",
	docs: `${DOCS}/VPC_CIDR_ALLOCATION.md`,
};

export const eventBusToggle: ConfigWarning = {
	code: "event_bus_toggle",
	item: { kind: "event_bus", name: "" },
	title: "event_bus.enabled changes from false to true",
	detail:
		"Turning the bus on creates it and its rules on the next deploy. Workloads that publish need their producers listed.",
};

export const unnamedService: ConfigWarning = {
	code: "service_no_health_check",
	item: { kind: "service", name: "#3" },
	title: "service '#3' is network_access: none and has no health_check_command",
	detail:
		"The third service has no name, so it has no node on the canvas. Give it a name and a health_check_command.",
};

/** A kind a newer server might send; the UI lists it with the env-wide ones. */
export const unknownKind = {
	code: "future_rule",
	item: { kind: "future_kind", name: "x" },
	title: "a warning of a kind this UI does not know yet",
	detail: "Shown with the environment warnings so it is never hidden.",
} as unknown as ConfigWarning;

/** A docs value that is not https is shown as text only, never linked. */
export const unsafeDocs: ConfigWarning = {
	code: "valkey_no_access",
	item: { kind: "valkey", name: "" },
	title: "valkey is enabled but no workload can reach it",
	detail: "Add the workloads that use the cache to valkey.access.",
	docs: "http://example.com/valkey",
};

export const fullReport: ConfigWarning[] = [
	vpcCidrOverlap,
	backendExternalImage,
	eventBusToggle,
	eventTaskProducesAny,
	servicePolicyWildcard,
	serviceNoHealthCheck,
];

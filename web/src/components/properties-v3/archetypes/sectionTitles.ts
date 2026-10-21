/**
 * properties-v3 — canonical section titles.
 *
 * The canonical section KEY owns the displayed heading across ALL archetypes
 * (fix for duplicate headings when a panel declares several groups in the
 * same section, e.g. PostgreSQL identity + endpoints). Archetypes compose
 * controls; they never hardcode category names.
 */

import type { ClosedSectionKey } from "../types";

export const SECTION_TITLES: Readonly<Record<ClosedSectionKey, string>> = {
	// Overview
	identity: "Identity",
	endpoints: "Endpoints",
	relationships: "Relationships",
	"current-state": "Current state",
	// Configuration
	"naming-lifecycle": "Naming & lifecycle",
	source: "Source",
	"compute-scaling": "Compute & scaling",
	"network-routing": "Network & routing",
	"storage-data": "Storage & data",
	"security-access": "Security & access",
	"environment-secrets": "Environment & secrets",
	observability: "Observability",
	advanced: "Advanced",
	// Operations
	deployments: "Deployments",
	"health-metrics": "Health & metrics",
	actions: "Actions",
	// Logs
	"log-viewer": "Log viewer",
	destination: "Destination",
	retention: "Retention",
	// Layout
	layout: "Layout",
};

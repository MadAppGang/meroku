/**
 * properties-v3 — a catalog panel bound to the environment YAML.
 *
 * Edits stay in the V3 draft until the user presses Save; replacement
 * (TF-REPLACE) changes go through the destructive confirmation first. Only
 * then is `onConfigChange` called — once per save, with the reviewed values.
 */

import { useMemo, useRef } from "react";
import type { YamlInfrastructureConfig } from "../../../types/yamlConfig";
import { NODE_CATALOG_BY_ID_V3 } from "../nodeCatalog";
import { PropertyPanel } from "../PropertyPanel";
import type { PanelDraftAdapters } from "../usePanelDraftState";
import {
	bindDefinitionResources,
	buildConfigInstance,
	type ConfigPanelBinding,
	configUpdatesFromChanges,
} from "./configAdapter";

export interface ConfigPropertyPanelProps {
	binding: ConfigPanelBinding;
	config: YamlInfrastructureConfig;
	onConfigChange: (updates: Partial<YamlInfrastructureConfig>) => void;
	onClose?: () => void;
}

export function ConfigPropertyPanel({
	binding,
	config,
	onConfigChange,
	onClose,
}: ConfigPropertyPanelProps) {
	const catalogDefinition = NODE_CATALOG_BY_ID_V3[binding.definitionId];
	if (!catalogDefinition) {
		throw new Error(`No properties-v3 definition "${binding.definitionId}"`);
	}

	const instance = useMemo(
		() => buildConfigInstance(catalogDefinition, config, binding),
		[catalogDefinition, config, binding],
	);
	const resourceId = instance.resource?.id;
	const resourceName = instance.resource?.name;
	const definition = useMemo(
		() =>
			resourceId && resourceName
				? bindDefinitionResources(catalogDefinition, {
						id: resourceId,
						name: resourceName,
					})
				: catalogDefinition,
		[catalogDefinition, resourceId, resourceName],
	);

	// Saves translate against the config current at save time, not at render.
	const configRef = useRef(config);
	configRef.current = config;

	const adapters = useMemo<PanelDraftAdapters>(
		() => ({
			onSaveChannel: ({ channel, changes }) => {
				if (channel !== "configuration") {
					return {
						ok: false,
						message: `${channel} changes are not available in this app yet`,
					};
				}
				const result = configUpdatesFromChanges(
					definition,
					configRef.current,
					binding,
					changes,
				);
				if (!result.ok) return result;
				onConfigChange(result.updates);
				return { ok: true };
			},
		}),
		[definition, binding, onConfigChange],
	);

	return (
		<PropertyPanel
			definition={definition}
			instance={instance}
			adapters={adapters}
			theme="dark"
			onClose={onClose}
		/>
	);
}

export default ConfigPropertyPanel;

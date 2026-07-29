import { describe, expect, test } from "bun:test";
import type { YamlInfrastructureConfig } from "../src/types/yamlConfig";
import {
	getAdditionalAlbDomain,
	getApiDomainUrl,
	getEnvironmentDomain,
} from "../src/utils/apiDomain";

function config(
	overrides: Partial<YamlInfrastructureConfig> = {},
): YamlInfrastructureConfig {
	return {
		project: "meroku",
		env: "dev",
		is_prod: false,
		region: "ap-southeast-2",
		state_bucket: "state",
		state_file: "dev.tfstate",
		domain: {
			enabled: true,
			domain_name: "example.com",
			api_domain_prefix: "api",
		},
		...overrides,
	};
}

describe("environment domain resolution", () => {
	test("adds the environment prefix for standard non-production domains", () => {
		expect(getEnvironmentDomain(config())).toBe("dev.example.com");
		expect(getApiDomainUrl(config())).toBe("api.dev.example.com");
	});

	test("honors an explicitly disabled prefix in non-production", () => {
		expect(
			getApiDomainUrl(
				config({
					domain: {
						enabled: true,
						domain_name: "example.com",
						api_domain_prefix: "api",
						add_env_domain_prefix: false,
					},
				}),
			),
		).toBe("api.example.com");
	});

	test("uses the environment domain when the API prefix is empty", () => {
		expect(
			getApiDomainUrl(
				config({
					domain: {
						enabled: true,
						domain_name: "example.com",
						api_domain_prefix: "",
					},
				}),
			),
		).toBe("dev.example.com");
	});

	test("never prefixes production", () => {
		expect(
			getApiDomainUrl(
				config({
					env: "prod",
					is_prod: true,
					domain: {
						enabled: true,
						domain_name: "example.com",
						api_domain_prefix: "api",
						add_env_domain_prefix: true,
					},
				}),
			),
		).toBe("api.example.com");
	});

	test("mirrors delegated non-production zones", () => {
		expect(
			getEnvironmentDomain(
				config({
					domain: {
						enabled: true,
						domain_name: "example.com",
						root_zone_id: "Z123",
						add_env_domain_prefix: false,
					},
				}),
			),
		).toBe("dev.example.com");
	});

	test("qualifies the optional ALB hostname with the resolved domain", () => {
		expect(
			getAdditionalAlbDomain(
				config({
					workload: { backend_alb_domain_name: "stream" },
				}),
			),
		).toBe("stream.dev.example.com");
	});

	test("returns no hostname when the domain module is disabled", () => {
		const disabled = config({
			domain: { enabled: false, domain_name: "example.com" },
		});
		expect(getEnvironmentDomain(disabled)).toBe("");
		expect(getApiDomainUrl(disabled)).toBe("");
		expect(
			getAdditionalAlbDomain({
				...disabled,
				workload: { backend_alb_domain_name: "stream" },
			}),
		).toBe("");
	});
});

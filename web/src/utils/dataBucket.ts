import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { BACKEND_WORKLOAD_NAME } from "./workloadReferences";

/** The data bucket's name, as the Data bucket tab shows it. */
export function dataBucketName(config: YamlInfrastructureConfig): string {
	return `${config.project}-data-${config.env}-${config.account_id || "<account>"}`;
}

/** The backend bucket's name, as modules/workloads creates it. */
export function backendBucketName(config: YamlInfrastructureConfig): string {
	return `${config.project}-backend-${config.env}${config.workload?.bucket_postfix || ""}`;
}

/** One statement a workload's task role gets, for display. */
export interface BucketGrant {
	name: string;
	actions: string[];
	resources: string[];
	description: string;
}

/**
 * The S3 grants modules/data_bucket puts on a workload's task role: its own
 * folder and the shared folders it is listed on, read-write or read, a list
 * scoped to those prefixes, and, for every workload except the backend,
 * object-level access to the backend bucket. The backend's own backend-bucket
 * access is the s3:* policy modules/workloads attaches.
 */
export function workloadBucketGrants(
	config: YamlInfrastructureConfig,
	workload: string,
): BucketGrant[] {
	const data = `arn:aws:s3:::${dataBucketName(config)}`;
	const shared = [...(config.data_bucket?.shared ?? [])].sort((a, b) =>
		a.name.localeCompare(b.name),
	);
	const readWrite = shared
		.filter((f) => f.read_write?.includes(workload))
		.map((f) => f.name);
	const readOnly = shared
		.filter(
			(f) => f.read?.includes(workload) && !f.read_write?.includes(workload),
		)
		.map((f) => f.name);

	const grants: BucketGrant[] = [
		{
			name: "Data Bucket Folders",
			actions: [
				"s3:GetObject",
				"s3:PutObject",
				"s3:DeleteObject",
				"s3:AbortMultipartUpload",
			],
			resources: [
				`${data}/${workload}/*`,
				...readWrite.map((f) => `${data}/shared/${f}/*`),
			],
			description:
				"Read and write its own folder (DATA_BUCKET_PATH) and the shared folders it may write",
		},
	];
	if (readOnly.length > 0) {
		grants.push({
			name: "Data Bucket Shared Folders (read)",
			actions: ["s3:GetObject"],
			resources: readOnly.map((f) => `${data}/shared/${f}/*`),
			description: "Read the shared folders it is listed on as a reader",
		});
	}
	grants.push({
		name: "Data Bucket List",
		actions: ["s3:ListBucket"],
		resources: [data],
		description: `Lists only under ${[`${workload}/`, ...[...readWrite, ...readOnly].map((f) => `shared/${f}/`)].join(", ")}`,
	});

	const backend = `arn:aws:s3:::${backendBucketName(config)}`;
	if (workload === BACKEND_WORKLOAD_NAME) {
		grants.unshift({
			name: "S3 Backend Bucket Access",
			actions: ["s3:*"],
			resources: [backend, `${backend}/*`],
			description: "Full access to the backend bucket",
		});
	} else {
		grants.unshift({
			name: "S3 Backend Bucket Objects",
			actions: [
				"s3:GetObject",
				"s3:PutObject",
				"s3:DeleteObject",
				"s3:AbortMultipartUpload",
				"s3:ListMultipartUploadParts",
				"s3:PutObjectAcl",
				"s3:ListBucket",
				"s3:GetBucketLocation",
			],
			resources: [backend, `${backend}/*`],
			description:
				"Object-level read and write on the backend bucket, and listing it; no bucket settings",
		});
	}
	return grants;
}

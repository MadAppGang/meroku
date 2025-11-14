import {
	AlertCircle,
	Copy,
	Database,
	Eye,
	EyeOff,
	FileText,
	Info,
	Key,
	RefreshCw,
	Server,
	Zap,
} from "lucide-react";
import { useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";

interface PostgresNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	accountInfo?: { accountId: string; region: string; profile: string };
}

export function PostgresNodeProperties({
	config,
	onConfigChange,
}: PostgresNodePropertiesProps) {
	const postgresConfig = config.postgres || { enabled: false };
	const workloadConfig = config.workload || {};
	// Remove unused password state variables that are only needed in PostgresConnectionInfo component

	// Aurora capacity validation state
	const [capacityWarning, setCapacityWarning] = useState<string | null>(null);

	// Validate Aurora capacity configuration
	const validateCapacity = (min: number, max: number): string | null => {
		// Rule 1: max must be >= 1
		if (max < 1) {
			return "Maximum capacity must be at least 1 ACU (AWS requirement)";
		}

		// Rule 2: min and max cannot both be 0.5
		if (min === 0.5 && max === 0.5) {
			return "Minimum and maximum cannot both be 0.5 ACU. Set maximum to 1 or higher.";
		}

		// Rule 3: max must be >= min
		if (max < min) {
			return "Maximum capacity must be greater than or equal to minimum capacity";
		}

		return null;
	};

	const handleTogglePostgres = (enabled: boolean) => {
		onConfigChange({
			postgres: {
				...postgresConfig,
				enabled,
			},
		});
	};

	const handleUpdateConfig = (updates: Partial<typeof postgresConfig>) => {
		onConfigChange({
			postgres: {
				...postgresConfig,
				...updates,
			},
		});
	};

	const handleTogglePgAdmin = (enabled: boolean) => {
		onConfigChange({
			workload: {
				...workloadConfig,
				install_pg_admin: enabled,
			},
		});
	};

	const handleUpdatePgAdminEmail = (email: string) => {
		onConfigChange({
			workload: {
				...workloadConfig,
				pg_admin_email: email,
			},
		});
	};

	// Password fetching moved to PostgresConnectionInfo component

	const [pgAdminPasswordVisible, setPgAdminPasswordVisible] = useState(false);
	const [pgAdminPasswordLoading, setPgAdminPasswordLoading] = useState(false);
	const [pgAdminPasswordValue, setPgAdminPasswordValue] = useState<
		string | null
	>(null);
	const [pgAdminPasswordError, setPgAdminPasswordError] = useState<
		string | null
	>(null);

	const fetchPgAdminPassword = async () => {
		setPgAdminPasswordLoading(true);
		setPgAdminPasswordError(null);

		try {
			const parameter = await infrastructureApi.getSSMParameter(
				`/${config.env}/${config.project}/pgadmin_password`,
			);
			setPgAdminPasswordValue(parameter.value);
		} catch (error: any) {
			setPgAdminPasswordError(
				error.message || "Failed to fetch pgAdmin password",
			);
		} finally {
			setPgAdminPasswordLoading(false);
		}
	};

	const copyToClipboard = (text: string) => {
		navigator.clipboard.writeText(text);
	};

	// These values are now used only in PostgresConnectionInfo component

	return (
		<div className="space-y-6">
			{/* Enable/Disable PostgreSQL */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Database className="w-5 h-5" />
						PostgreSQL Database
					</CardTitle>
					<CardDescription>
						{postgresConfig.aurora
							? "AWS Aurora PostgreSQL Serverless v2 cluster"
							: "AWS RDS PostgreSQL database instance"}
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="flex items-center justify-between">
						<div className="space-y-1">
							<Label htmlFor="postgres-enabled" className="text-base">
								Enable PostgreSQL
							</Label>
							<p className="text-sm text-gray-500">
								Create a managed PostgreSQL database cluster
							</p>
						</div>
						<Switch
							id="postgres-enabled"
							checked={postgresConfig.enabled}
							onCheckedChange={handleTogglePostgres}
						/>
					</div>
				</CardContent>
			</Card>

			{postgresConfig.enabled && (
				<>
					{/* Database Configuration */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Server className="w-5 h-5" />
								Database Configuration
							</CardTitle>
							<CardDescription>
								Configure your PostgreSQL database settings
							</CardDescription>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
								<div className="space-y-1">
									<Label className="text-sm font-medium flex items-center gap-2">
										<Zap className="w-4 h-4 text-yellow-400" />
										Aurora Serverless v2
									</Label>
									<p className="text-xs text-gray-400">
										Use auto-scaling Aurora Serverless v2 instead of standard
										RDS
									</p>
								</div>
								<Switch
									checked={postgresConfig.aurora || false}
									onCheckedChange={(checked) =>
										handleUpdateConfig({ aurora: checked })
									}
								/>
							</div>

							{postgresConfig.aurora && (
								<div className="grid grid-cols-2 gap-4 p-3 bg-gray-800 rounded-lg">
									<div className="space-y-2">
										<Label htmlFor="min-capacity">Min Capacity (ACUs)</Label>
										<Input
											id="min-capacity"
											type="number"
											step="0.5"
											min="0"
											max="128"
											value={postgresConfig.min_capacity ?? 0}
											onChange={(e) => {
												const newMin = parseFloat(e.target.value);
												const newMax = postgresConfig.max_capacity || 1;

												// Auto-correct max if it would be invalid
												const correctedMax =
													newMin === 0.5 && newMax === 0.5 ? 1 : newMax;

												handleUpdateConfig({
													min_capacity: newMin,
													...(correctedMax !== newMax && {
														max_capacity: correctedMax,
													}),
												});

												setCapacityWarning(validateCapacity(newMin, correctedMax));
											}}
										/>
										<p className="text-xs text-gray-500">
											Minimum: 0 ACU (enables automatic pause when idle)
										</p>
									</div>

									<div className="space-y-2">
										<Label htmlFor="max-capacity">Max Capacity (ACUs)</Label>
										<Input
											id="max-capacity"
											type="number"
											step="0.5"
											min="1"
											max="128"
											value={postgresConfig.max_capacity || 1}
											onChange={(e) => {
												const newMax = Math.max(1, parseFloat(e.target.value)); // Always >= 1
												const newMin = postgresConfig.min_capacity ?? 0;

												handleUpdateConfig({
													max_capacity: newMax,
												});

												setCapacityWarning(validateCapacity(newMin, newMax));
											}}
										/>
										<p className="text-xs text-gray-500">
											Minimum: 1 ACU (AWS requirement), Maximum: 128 ACUs
										</p>
									</div>

									{capacityWarning && (
										<div className="col-span-2">
											<Alert className="border-yellow-600 bg-yellow-900/20">
												<AlertCircle className="h-4 w-4 text-yellow-400" />
												<AlertDescription className="text-yellow-200">
													{capacityWarning}
												</AlertDescription>
											</Alert>
										</div>
									)}

									<div className="col-span-2">
										<Alert>
											<Info className="h-4 w-4" />
											<AlertDescription>
												<strong>ACU (Aurora Capacity Unit):</strong> Each ACU
												provides 2 GiB of memory and corresponding compute.
												Aurora Serverless v2 automatically scales between your
												min and max capacity based on workload. Setting min
												capacity to 0 allows the database to pause when idle,
												significantly reducing costs.
											</AlertDescription>
										</Alert>
									</div>
								</div>
							)}

							{!postgresConfig.aurora && (
								<div className="space-y-4 p-3 bg-gray-800 rounded-lg">
									<h3 className="text-sm font-semibold text-gray-300 mb-3">
										RDS Instance Configuration
									</h3>

									{/* Instance Class Dropdown */}
									<div className="space-y-2">
										<Label htmlFor="instance-class">Instance Class</Label>
										<select
											id="instance-class"
											value={postgresConfig.instance_class || "db.t4g.micro"}
											onChange={(e) =>
												handleUpdateConfig({ instance_class: e.target.value })
											}
											className="w-full px-3 py-2 bg-gray-900 border border-gray-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
										>
											<optgroup label="T4g - Burstable (ARM, Cost-effective)">
												<option value="db.t4g.micro">
													db.t4g.micro (2 vCPU, 1GB) - ~$12/mo
												</option>
												<option value="db.t4g.small">
													db.t4g.small (2 vCPU, 2GB) - ~$23/mo
												</option>
												<option value="db.t4g.medium">
													db.t4g.medium (2 vCPU, 4GB) - ~$47/mo
												</option>
												<option value="db.t4g.large">
													db.t4g.large (2 vCPU, 8GB) - ~$94/mo
												</option>
											</optgroup>
											<optgroup label="M6i - Balanced Production">
												<option value="db.m6i.large">
													db.m6i.large (2 vCPU, 8GB) - ~$130/mo
												</option>
												<option value="db.m6i.xlarge">
													db.m6i.xlarge (4 vCPU, 16GB) - ~$260/mo
												</option>
												<option value="db.m6i.2xlarge">
													db.m6i.2xlarge (8 vCPU, 32GB) - ~$520/mo
												</option>
											</optgroup>
											<optgroup label="R6i - Memory-Optimized">
												<option value="db.r6i.large">
													db.r6i.large (2 vCPU, 16GB) - ~$175/mo
												</option>
												<option value="db.r6i.xlarge">
													db.r6i.xlarge (4 vCPU, 32GB) - ~$350/mo
												</option>
											</optgroup>
										</select>
										<p className="text-xs text-gray-500">
											Choose instance size based on your workload requirements
										</p>
									</div>

									{/* Storage Size */}
									<div className="space-y-2">
										<Label htmlFor="allocated-storage">
											Allocated Storage (GB)
										</Label>
										<Input
											id="allocated-storage"
											type="number"
											min={20}
											max={65536}
											value={postgresConfig.allocated_storage || 20}
											onChange={(e) =>
												handleUpdateConfig({
													allocated_storage: Number(e.target.value),
												})
											}
										/>
										<p className="text-xs text-gray-500">
											gp3 SSD - $0.115/GB/month (20 GB minimum, 65,536 GB
											maximum)
										</p>
									</div>

									{/* Multi-AZ Toggle */}
									<div className="flex items-center justify-between p-3 bg-gray-900 rounded-lg">
										<div className="space-y-1">
											<Label className="text-sm font-medium">
												Multi-AZ Deployment
											</Label>
											<p className="text-xs text-gray-400">
												High availability with automatic failover (doubles
												instance cost)
											</p>
										</div>
										<Switch
											checked={postgresConfig.multi_az || false}
											onCheckedChange={(checked) =>
												handleUpdateConfig({ multi_az: checked })
											}
										/>
									</div>

									{/* Storage Encryption */}
									<div className="flex items-center justify-between p-3 bg-gray-900 rounded-lg">
										<div className="space-y-1">
											<Label className="text-sm font-medium">
												Storage Encryption
											</Label>
											<p className="text-xs text-gray-400">
												Encrypt data at rest (recommended)
											</p>
										</div>
										<Switch
											checked={
												postgresConfig.storage_encrypted !== false
											}
											onCheckedChange={(checked) =>
												handleUpdateConfig({ storage_encrypted: checked })
											}
										/>
									</div>

									{/* Deletion Protection */}
									<div className="flex items-center justify-between p-3 bg-gray-900 rounded-lg">
										<div className="space-y-1">
											<Label className="text-sm font-medium">
												Deletion Protection
											</Label>
											<p className="text-xs text-gray-400">
												Prevent accidental database deletion
											</p>
										</div>
										<Switch
											checked={postgresConfig.deletion_protection || false}
											onCheckedChange={(checked) =>
												handleUpdateConfig({ deletion_protection: checked })
											}
										/>
									</div>

									{/* Skip Final Snapshot */}
									<div className="flex items-center justify-between p-3 bg-gray-900 rounded-lg">
										<div className="space-y-1">
											<Label className="text-sm font-medium">
												Skip Final Snapshot
											</Label>
											<p className="text-xs text-gray-400">
												Skip creating snapshot when deleting (not recommended
												for production)
											</p>
										</div>
										<Switch
											checked={
												postgresConfig.skip_final_snapshot !== false
											}
											onCheckedChange={(checked) =>
												handleUpdateConfig({ skip_final_snapshot: checked })
											}
										/>
									</div>

									{/* Cost Estimate */}
									<div className="p-3 bg-blue-900/20 border border-blue-700 rounded-lg">
										<h4 className="text-sm font-semibold text-blue-300 mb-2">
											💰 Estimated Monthly Cost
										</h4>
										<p className="text-2xl font-bold text-white mb-2">
											${(() => {
												const instancePrices: Record<string, number> = {
													"db.t4g.micro": 11.68,
													"db.t4g.small": 23.36,
													"db.t4g.medium": 47.45,
													"db.t4g.large": 94.17,
													"db.m6i.large": 129.94,
													"db.m6i.xlarge": 259.88,
													"db.m6i.2xlarge": 519.76,
													"db.r6i.large": 175.2,
													"db.r6i.xlarge": 350.4,
												};

												const instanceClass =
													postgresConfig.instance_class || "db.t4g.micro";
												let instanceCost = instancePrices[instanceClass] || 23.36;

												if (postgresConfig.multi_az) {
													instanceCost *= 2;
												}

												const storage = postgresConfig.allocated_storage || 20;
												const storageCost = storage * 0.115;

												return (instanceCost + storageCost).toFixed(2);
											})()}
											/month
										</p>
										<div className="text-xs text-gray-400 space-y-1">
											<div>
												Instance:{" "}
												{postgresConfig.instance_class || "db.t4g.micro"}{" "}
												{postgresConfig.multi_az && "(Multi-AZ)"} - $
												{(() => {
													const instancePrices: Record<string, number> = {
														"db.t4g.micro": 11.68,
														"db.t4g.small": 23.36,
														"db.t4g.medium": 47.45,
														"db.t4g.large": 94.17,
														"db.m6i.large": 129.94,
														"db.m6i.xlarge": 259.88,
														"db.m6i.2xlarge": 519.76,
														"db.r6i.large": 175.2,
														"db.r6i.xlarge": 350.4,
													};
													const instanceClass =
														postgresConfig.instance_class || "db.t4g.micro";
													let cost = instancePrices[instanceClass] || 23.36;
													if (postgresConfig.multi_az) cost *= 2;
													return cost.toFixed(2);
												})()}
												/mo
											</div>
											<div>
												Storage: {postgresConfig.allocated_storage || 20}GB gp3
												- $
												{((postgresConfig.allocated_storage || 20) * 0.115).toFixed(
													2,
												)}
												/mo
											</div>
										</div>
									</div>
								</div>
							)}

							<div className="grid grid-cols-2 gap-4">
								<div className="space-y-2">
									<Label htmlFor="db-name">Database Name</Label>
									<Input
										id="db-name"
										value={postgresConfig.dbname || ""}
										onChange={(e) =>
											handleUpdateConfig({ dbname: e.target.value })
										}
										placeholder={config.project}
									/>
									<p className="text-xs text-gray-500">
										Default: {config.project}
									</p>
								</div>

								<div className="space-y-2">
									<Label htmlFor="db-username">Master Username</Label>
									<Input
										id="db-username"
										value={postgresConfig.username || ""}
										onChange={(e) =>
											handleUpdateConfig({ username: e.target.value })
										}
										placeholder="postgres"
									/>
									<p className="text-xs text-gray-500">Default: postgres</p>
								</div>
							</div>

							<div className="space-y-2">
								<Label htmlFor="engine-version">PostgreSQL Version</Label>
								<select
									id="engine-version"
									value={postgresConfig.engine_version || "16"}
									onChange={(e) =>
										handleUpdateConfig({ engine_version: e.target.value })
									}
									className="w-full px-3 py-2 bg-gray-800 border border-gray-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
								>
									<option value="17">PostgreSQL 17.x (Latest)</option>
									<option value="16">PostgreSQL 16.x</option>
									<option value="15">PostgreSQL 15.x</option>
									<option value="14">PostgreSQL 14.x</option>
									<option value="13">PostgreSQL 13.x</option>
								</select>
								{postgresConfig.aurora && (
									<p className="text-xs text-gray-500">
										Aurora Serverless v2 supports PostgreSQL 13.x through 17.x
									</p>
								)}
							</div>

							<div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
								<div className="space-y-1">
									<Label className="text-sm font-medium">Public Access</Label>
									<p className="text-xs text-gray-400">
										Allow connections from outside VPC
									</p>
								</div>
								<Switch
									checked={postgresConfig.public_access || false}
									onCheckedChange={(checked) =>
										handleUpdateConfig({ public_access: checked })
									}
								/>
							</div>

							{postgresConfig.public_access && (
								<Alert className="border-yellow-600 bg-yellow-50">
									<AlertCircle className="h-4 w-4 text-yellow-600" />
									<AlertDescription>
										Enabling public access exposes your database to the
										internet. Ensure proper security groups and strong
										passwords.
									</AlertDescription>
								</Alert>
							)}

							<div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
								<div className="space-y-1">
									<Label className="text-sm font-medium flex items-center gap-2">
										<Key className="w-4 h-4 text-blue-400" />
										IAM Database Authentication
									</Label>
									<p className="text-xs text-gray-400">
										Use IAM roles for database authentication instead of passwords
									</p>
								</div>
								<Switch
									checked={postgresConfig.iam_database_authentication_enabled || false}
									onCheckedChange={(checked) =>
										handleUpdateConfig({
											iam_database_authentication_enabled: checked,
										})
									}
								/>
							</div>

							{postgresConfig.iam_database_authentication_enabled && (
								<Alert className="border-blue-600 bg-blue-900/20">
									<Info className="h-4 w-4 text-blue-400" />
									<AlertDescription className="text-blue-200">
										<strong>IAM Authentication enabled.</strong> You can use AWS IAM roles
										to manage database access. ECS tasks will be able to authenticate using
										their IAM role instead of database passwords.
									</AlertDescription>
								</Alert>
							)}
						</CardContent>
					</Card>

					{/* pgAdmin Configuration */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<FileText className="w-5 h-5" />
								pgAdmin Interface
							</CardTitle>
							<CardDescription>
								Web-based PostgreSQL administration tool
							</CardDescription>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
								<div className="space-y-1">
									<Label className="text-sm font-medium">Enable pgAdmin</Label>
									<p className="text-xs text-gray-400">
										Deploy pgAdmin container in ECS
									</p>
								</div>
								<Switch
									checked={workloadConfig.install_pg_admin || false}
									onCheckedChange={handleTogglePgAdmin}
								/>
							</div>

							{workloadConfig.install_pg_admin && (
								<div className="space-y-4">
									<div className="space-y-2">
										<Label htmlFor="pgadmin-email">pgAdmin Login Email</Label>
										<Input
											id="pgadmin-email"
											type="email"
											value={workloadConfig.pg_admin_email || ""}
											onChange={(e) => handleUpdatePgAdminEmail(e.target.value)}
											placeholder="admin@example.com"
										/>
										<p className="text-xs text-gray-500">
											Default: admin@madappgang.com
										</p>
									</div>

									<div className="space-y-2">
										<Label>pgAdmin Password</Label>
										<div className="bg-gray-800 rounded-lg p-3">
											<p className="text-xs text-gray-500 mb-3">
												Stored in Parameter Store:{" "}
												<code>
													/{config.env}/{config.project}/pgadmin_password
												</code>
											</p>

											<div className="flex items-center gap-2 mb-2">
												<Button
													size="sm"
													variant="outline"
													onClick={fetchPgAdminPassword}
													disabled={pgAdminPasswordLoading}
													className="text-xs"
												>
													{pgAdminPasswordLoading ? (
														<>
															<RefreshCw className="w-3 h-3 mr-1 animate-spin" />
															Loading...
														</>
													) : (
														<>
															<Key className="w-3 h-3 mr-1" />
															Fetch Password
														</>
													)}
												</Button>

												{pgAdminPasswordValue && (
													<>
														<Button
															size="sm"
															variant="ghost"
															onClick={() =>
																setPgAdminPasswordVisible(
																	!pgAdminPasswordVisible,
																)
															}
															className="text-xs"
														>
															{pgAdminPasswordVisible ? (
																<EyeOff className="w-3 h-3" />
															) : (
																<Eye className="w-3 h-3" />
															)}
														</Button>

														<Button
															size="sm"
															variant="ghost"
															onClick={() =>
																copyToClipboard(pgAdminPasswordValue)
															}
															className="text-xs"
														>
															<Copy className="w-3 h-3" />
														</Button>
													</>
												)}
											</div>

											{pgAdminPasswordError && (
												<div className="text-xs text-red-400 bg-red-900/20 border border-red-700 rounded p-2 mb-2">
													{pgAdminPasswordError}
												</div>
											)}

											{pgAdminPasswordValue && (
												<div className="text-xs bg-gray-900 p-2 rounded border">
													{pgAdminPasswordVisible ? (
														<span className="font-mono text-green-400">
															{pgAdminPasswordValue}
														</span>
													) : (
														<span className="font-mono text-gray-400">
															{"•".repeat(20)}
														</span>
													)}
												</div>
											)}
										</div>
									</div>
								</div>
							)}
						</CardContent>
					</Card>
				</>
			)}
		</div>
	);
}

import {
	Copy,
	Database,
	Eye,
	EyeOff,
	FileCode,
	Info,
	Key,
	Lock,
	RefreshCw,
	Server,
	Shield,
	Terminal,
} from "lucide-react";
import { useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";

interface PostgresConnectionInfoProps {
	config: YamlInfrastructureConfig;
}

export function PostgresConnectionInfo({
	config,
}: PostgresConnectionInfoProps) {
	const postgresConfig = config.postgres || { enabled: false };
	const actualDbName = postgresConfig.dbname || config.project;
	const actualUsername = postgresConfig.username || "postgres";

	const [passwordVisible, setPasswordVisible] = useState(false);
	const [passwordLoading, setPasswordLoading] = useState(false);
	const [passwordValue, setPasswordValue] = useState<string | null>(null);
	const [passwordError, setPasswordError] = useState<string | null>(null);

	const [hostValue, setHostValue] = useState<string | null>(null);
	const [hostLoading, setHostLoading] = useState(false);
	const [hostError, setHostError] = useState<string | null>(null);

	const [connectionStringVisible, setConnectionStringVisible] = useState(false);

	const fetchPassword = async () => {
		setPasswordLoading(true);
		setPasswordError(null);

		try {
			const parameter = await infrastructureApi.getSSMParameter(
				`/${config.env}/${config.project}/postgres_password`,
			);
			setPasswordValue(parameter.value);
		} catch (error: any) {
			setPasswordError(error.message || "Failed to fetch password");
		} finally {
			setPasswordLoading(false);
		}
	};

	const fetchHost = async () => {
		setHostLoading(true);
		setHostError(null);

		try {
			// Fetch database info directly from AWS RDS/Aurora API
			const dbInfo = await infrastructureApi.getDatabaseInfo(
				config.project,
				config.env,
			);

			if (dbInfo.endpoint) {
				setHostValue(dbInfo.endpoint);
				// Also set the port if it differs from default
				if (dbInfo.port && dbInfo.port !== 5432) {
					setHostValue(`${dbInfo.endpoint}:${dbInfo.port}`);
				}
			} else {
				throw new Error("No endpoint found in database info");
			}
		} catch (error: any) {
			setHostError(error.message || "Failed to fetch database endpoint");
			// Fallback to a reasonable default pattern
			const defaultHost = postgresConfig.aurora
				? `${config.project}-aurora-${config.env}.cluster-xxxxx.${config.region}.rds.amazonaws.com`
				: `${config.project}-postgres-${config.env}.xxxxx.${config.region}.rds.amazonaws.com`;
			setHostValue(defaultHost);
		} finally {
			setHostLoading(false);
		}
	};

	const buildConnectionString = () => {
		if (passwordValue && hostValue) {
			// Check if port is already included in hostValue
			const hasPort = hostValue.includes(":");
			const connectionHost = hasPort ? hostValue : `${hostValue}:5432`;
			return `postgresql://${actualUsername}:${passwordValue}@${connectionHost}/${actualDbName}`;
		}
		return null;
	};

	const copyToClipboard = (text: string) => {
		navigator.clipboard.writeText(text);
	};

	if (!postgresConfig.enabled) {
		return (
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Database className="w-5 h-5" />
						Database Connection
					</CardTitle>
				</CardHeader>
				<CardContent>
					<Alert>
						<Info className="h-4 w-4" />
						<AlertDescription>
							PostgreSQL is not enabled. Enable it in the Settings tab to see
							connection information.
						</AlertDescription>
					</Alert>
				</CardContent>
			</Card>
		);
	}

	return (
		<div className="space-y-6">
			{/* Connection Overview */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Database className="w-5 h-5" />
						Database Connection Overview
					</CardTitle>
					<CardDescription>
						How your backend service connects to{" "}
						{postgresConfig.aurora ? "Aurora Serverless v2" : "RDS PostgreSQL"}
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="bg-gray-800 rounded-lg p-4">
						<h4 className="text-sm font-medium text-gray-200 mb-3 flex items-center gap-2">
							<Server className="w-4 h-4" />
							Connection Architecture
						</h4>
						<div className="space-y-2 text-xs text-gray-300">
							<p>
								• Backend ECS tasks connect directly to the database endpoint
							</p>
							<p>• Connection is secured within VPC using security groups</p>
							<p>
								•{" "}
								{postgresConfig.aurora
									? "Aurora endpoint automatically handles failover"
									: "RDS provides a single stable endpoint"}
							</p>
							<p>• Port 5432 (PostgreSQL standard port)</p>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Environment Variables */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Terminal className="w-5 h-5" />
						Environment Variables
					</CardTitle>
					<CardDescription>
						Variables automatically injected into your backend container
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="space-y-3">
						<div className="bg-gray-800 rounded-lg p-3">
							<div className="flex items-center justify-between mb-2">
								<code className="text-sm font-mono text-blue-400">
									PG_DATABASE_HOST
								</code>
								<Badge variant="outline" className="text-xs">
									Plaintext
								</Badge>
							</div>
							<p className="text-xs text-gray-400">
								Database endpoint from <code>module.postgres.endpoint</code>
							</p>
							<p className="text-xs text-gray-500 mt-1">
								{postgresConfig.aurora
									? "Aurora cluster endpoint"
									: "RDS instance address"}
							</p>

							{/* Host Viewer */}
							<div className="mt-3 pt-2 border-t border-gray-700">
								<div className="flex items-center gap-2 mb-2">
									<Button
										size="sm"
										variant="outline"
										onClick={fetchHost}
										disabled={hostLoading}
										className="text-xs"
									>
										{hostLoading ? (
											<>
												<RefreshCw className="w-3 h-3 mr-1 animate-spin" />
												Loading...
											</>
										) : (
											<>
												<Server className="w-3 h-3 mr-1" />
												Get Host
											</>
										)}
									</Button>

									{hostValue && (
										<Button
											size="sm"
											variant="ghost"
											onClick={() => copyToClipboard(hostValue)}
											className="text-xs"
										>
											<Copy className="w-3 h-3" />
										</Button>
									)}
								</div>

								{hostError && (
									<div className="text-xs text-yellow-400 bg-yellow-900/20 border border-yellow-700 rounded p-2 mb-2">
										{hostError}
									</div>
								)}

								{hostValue && (
									<div className="text-xs bg-gray-900 p-2 rounded border">
										<span className="font-mono text-green-400 break-all">
											{hostValue}
										</span>
									</div>
								)}
							</div>
						</div>

						<div className="bg-gray-800 rounded-lg p-3">
							<div className="flex items-center justify-between mb-2">
								<code className="text-sm font-mono text-blue-400">
									PG_DATABASE_USERNAME
								</code>
								<Badge variant="outline" className="text-xs">
									Plaintext
								</Badge>
							</div>
							<p className="text-xs text-gray-400">
								Value: <code className="text-green-400">{actualUsername}</code>
							</p>
						</div>

						<div className="bg-gray-800 rounded-lg p-3">
							<div className="flex items-center justify-between mb-2">
								<code className="text-sm font-mono text-blue-400">
									PG_DATABASE_NAME
								</code>
								<Badge variant="outline" className="text-xs">
									Plaintext
								</Badge>
							</div>
							<p className="text-xs text-gray-400">
								Value: <code className="text-green-400">{actualDbName}</code>
							</p>
						</div>

						<div className="bg-gray-800 rounded-lg p-3">
							<div className="flex items-center justify-between mb-2">
								<code className="text-sm font-mono text-blue-400">
									PG_DATABASE_PASSWORD
								</code>
								<Badge
									variant="secondary"
									className="text-xs bg-yellow-600/20 text-yellow-400 border-yellow-600/30"
								>
									<Lock className="w-3 h-3 mr-1" />
									Secure
								</Badge>
							</div>
							<p className="text-xs text-gray-400">
								Retrieved from SSM Parameter Store at runtime
							</p>
							<p className="text-xs text-gray-500 mt-1">
								Path:{" "}
								<code>
									/{config.env}/{config.project}/backend/pg_database_password
								</code>
							</p>
						</div>
					</div>

					<div className="bg-yellow-900/10 border border-yellow-600/30 rounded-lg p-3">
						<div className="flex items-start gap-2">
							<Info className="h-4 w-4 text-yellow-600 mt-0.5 flex-shrink-0" />
							<p className="text-xs text-gray-300">
								The password is injected securely using ECS secrets
								configuration with{" "}
								<code className="text-yellow-500">valueFrom</code> pointing to
								the SSM parameter. It never appears in logs or task definitions.
							</p>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Password Storage */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Key className="w-5 h-5" />
						Password Management
					</CardTitle>
					<CardDescription>
						How database passwords are generated and stored
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="space-y-3">
						<div className="bg-gray-800 rounded-lg p-3">
							<h4 className="text-sm font-medium text-gray-200 mb-2">
								Password Generation
							</h4>
							<ul className="space-y-1 text-xs text-gray-400">
								<li>
									• Automatically generated using Terraform's{" "}
									<code>random_password</code> resource
								</li>
								<li>• 16 characters with special characters</li>
								<li>• Generated once during infrastructure creation</li>
							</ul>
						</div>

						<div className="bg-gray-800 rounded-lg p-3">
							<h4 className="text-sm font-medium text-gray-200 mb-2">
								SSM Parameter Store Locations
							</h4>
							<div className="space-y-2">
								<div>
									<code className="text-xs text-blue-400">
										/{config.env}/{config.project}/postgres_password
									</code>
									<p className="text-xs text-gray-500 mt-1">
										Main password parameter
									</p>
								</div>
								<div>
									<code className="text-xs text-blue-400">
										/{config.env}/{config.project}/backend/pg_database_password
									</code>
									<p className="text-xs text-gray-500 mt-1">
										Backend service copy (used by ECS)
									</p>
								</div>
							</div>

							{/* Password Viewer */}
							<div className="mt-4 pt-3 border-t border-gray-700">
								<div className="flex items-center gap-2 mb-2">
									<Button
										size="sm"
										variant="outline"
										onClick={fetchPassword}
										disabled={passwordLoading}
										className="text-xs"
									>
										{passwordLoading ? (
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

									{passwordValue && (
										<Button
											size="sm"
											variant="ghost"
											onClick={() => setPasswordVisible(!passwordVisible)}
											className="text-xs"
										>
											{passwordVisible ? (
												<EyeOff className="w-3 h-3" />
											) : (
												<Eye className="w-3 h-3" />
											)}
										</Button>
									)}

									{passwordValue && (
										<Button
											size="sm"
											variant="ghost"
											onClick={() => copyToClipboard(passwordValue)}
											className="text-xs"
										>
											<Copy className="w-3 h-3" />
										</Button>
									)}
								</div>

								{passwordError && (
									<div className="text-xs text-red-400 bg-red-900/20 border border-red-700 rounded p-2 mb-2">
										{passwordError}
									</div>
								)}

								{passwordValue && (
									<div className="text-xs bg-gray-900 p-2 rounded border">
										{passwordVisible ? (
											<span className="font-mono text-green-400">
												{passwordValue}
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

						<div className="bg-gray-800 rounded-lg p-3">
							<h4 className="text-sm font-medium text-gray-200 mb-2 flex items-center gap-2">
								<Shield className="w-4 h-4 text-green-400" />
								Security Features
							</h4>
							<ul className="space-y-1 text-xs text-gray-400">
								<li>• Encrypted at rest using AWS KMS (SecureString type)</li>
								<li>• ECS task execution role has permission to decrypt</li>
								<li>• Password never appears in CloudWatch logs</li>
								<li>• Not stored in Terraform state</li>
							</ul>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Connection String */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<FileCode className="w-5 h-5" />
						Connection String Construction
					</CardTitle>
					<CardDescription>
						How to build the database connection URL in your application
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="bg-gray-800 rounded-lg p-4">
						<h4 className="text-sm font-medium text-gray-200 mb-3">
							Connection String Format
						</h4>
						<div className="bg-gray-900 rounded p-3 overflow-x-auto">
							<code className="text-xs text-green-400 whitespace-pre">
								{`postgresql://\${PG_DATABASE_USERNAME}:\${PG_DATABASE_PASSWORD}@\${PG_DATABASE_HOST}:5432/\${PG_DATABASE_NAME}`}
							</code>
						</div>

						{/* Interactive Connection String Builder */}
						<div className="mt-4 pt-3 border-t border-gray-700">
							<div className="flex items-center justify-between mb-2">
								<h5 className="text-xs font-medium text-gray-300">
									Build Full Connection String
								</h5>
								{(!hostValue || !passwordValue) && (
									<Button
										size="sm"
										variant="outline"
										onClick={async () => {
											if (!hostValue) await fetchHost();
											if (!passwordValue) await fetchPassword();
										}}
										disabled={hostLoading || passwordLoading}
										className="text-xs"
									>
										{hostLoading || passwordLoading ? (
											<>
												<RefreshCw className="w-3 h-3 mr-1 animate-spin" />
												Loading...
											</>
										) : (
											<>
												<Database className="w-3 h-3 mr-1" />
												Get Connection Details
											</>
										)}
									</Button>
								)}
							</div>

							{/* Show any errors */}
							{(passwordError || hostError) && (
								<div className="mb-2">
									{passwordError && (
										<div className="text-xs text-red-400 bg-red-900/20 border border-red-700 rounded p-2 mb-2">
											<strong>Password Error:</strong> {passwordError}
										</div>
									)}
									{hostError && hostValue && (
										<div className="text-xs text-yellow-400 bg-yellow-900/20 border border-yellow-700 rounded p-2">
											<strong>Host Notice:</strong> {hostError}
										</div>
									)}
								</div>
							)}

							{hostValue && passwordValue && (
								<>
									<div className="flex items-center gap-2 mb-2">
										<Button
											size="sm"
											variant="outline"
											onClick={() =>
												setConnectionStringVisible(!connectionStringVisible)
											}
											className="text-xs"
										>
											{connectionStringVisible ? (
												<>
													<EyeOff className="w-3 h-3 mr-1" /> Hide
												</>
											) : (
												<>
													<Eye className="w-3 h-3 mr-1" /> Show
												</>
											)}
										</Button>
										<Button
											size="sm"
											variant="ghost"
											onClick={() => {
												const connStr = buildConnectionString();
												if (connStr) copyToClipboard(connStr);
											}}
											className="text-xs"
										>
											<Copy className="w-3 h-3 mr-1" />
											Copy
										</Button>
									</div>

									{connectionStringVisible && (
										<div className="bg-gray-900 rounded p-3 overflow-x-auto">
											<code className="text-xs text-green-400 break-all">
												{buildConnectionString()}
											</code>
										</div>
									)}
								</>
							)}

							{(!hostValue || !passwordValue) &&
								!hostLoading &&
								!passwordLoading && (
									<div className="text-xs text-gray-500">
										Click "Get Connection Details" to build the connection
										string
									</div>
								)}
						</div>
					</div>

					<div className="bg-gray-800 rounded-lg p-4">
						<h4 className="text-sm font-medium text-gray-200 mb-3">
							Example in Node.js
						</h4>
						<div className="bg-gray-900 rounded p-3 overflow-x-auto">
							<pre className="text-xs text-gray-300">
								{`const dbConfig = {
  host: process.env.PG_DATABASE_HOST,
  port: 5432,
  database: process.env.PG_DATABASE_NAME,
  user: process.env.PG_DATABASE_USERNAME,
  password: process.env.PG_DATABASE_PASSWORD
};

// Or as a connection string
const connectionString = \`postgresql://\${process.env.PG_DATABASE_USERNAME}:\${process.env.PG_DATABASE_PASSWORD}@\${process.env.PG_DATABASE_HOST}:5432/\${process.env.PG_DATABASE_NAME}\`;`}
							</pre>
						</div>
					</div>

					<Alert>
						<Info className="h-4 w-4" />
						<AlertDescription className="text-xs">
							<strong>Note:</strong> While the UI mentions{" "}
							<code>DATABASE_URL</code>, the actual implementation provides
							separate environment variables. Your application needs to
							construct the connection string from these individual components.
						</AlertDescription>
					</Alert>
				</CardContent>
			</Card>

			{/* Network Security */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Shield className="w-5 h-5" />
						Network Security
					</CardTitle>
					<CardDescription>How database access is secured</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="space-y-3">
						<div className="bg-gray-800 rounded-lg p-3">
							<h4 className="text-sm font-medium text-gray-200 mb-2">
								Security Group Configuration
							</h4>
							<ul className="space-y-1 text-xs text-gray-400">
								<li>• Database has its own security group</li>
								<li>• Only allows inbound traffic on port 5432</li>
								<li>• Backend service security group is authorized</li>
								<li>
									•{" "}
									{postgresConfig.public_access ? (
										<span className="text-yellow-400">
											⚠️ Public access is enabled - database is accessible from
											internet
										</span>
									) : (
										<span className="text-green-400">
											✓ No public access - only VPC internal traffic
										</span>
									)}
								</li>
							</ul>
						</div>

						<div className="bg-gray-800 rounded-lg p-3">
							<h4 className="text-sm font-medium text-gray-200 mb-2">
								Additional Security
							</h4>
							<ul className="space-y-1 text-xs text-gray-400">
								<li>• SSL/TLS encryption for connections</li>
								<li>
									•{" "}
									{postgresConfig.aurora
										? "Aurora encryption at rest"
										: "RDS encryption at rest"}
								</li>
								<li>• Automated backups with encryption</li>
								<li>• IAM role-based access for ECS tasks</li>
							</ul>
						</div>
					</div>
				</CardContent>
			</Card>
		</div>
	);
}

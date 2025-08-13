import { useCallback, useEffect, useState } from "react";
import { type Environment, infrastructureApi } from "../api/infrastructure";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./ui/tabs";
import { Textarea } from "./ui/textarea";

interface EnvironmentConfigProps {
	selectedEnvironment: string | null;
}

export function EnvironmentConfig({
	selectedEnvironment,
}: EnvironmentConfigProps) {
	const [environments, setEnvironments] = useState<Environment[]>([]);
	const [selectedEnv, setSelectedEnv] = useState<string>("");
	const [configContent, setConfigContent] = useState<string>("");
	const [isLoading, setIsLoading] = useState(false);
	const [isSaving, setIsSaving] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const [successMessage, setSuccessMessage] = useState<string | null>(null);

	const loadEnvironments = useCallback(async () => {
		try {
			setIsLoading(true);
			setError(null);
			const envs = await infrastructureApi.getEnvironments();
			setEnvironments(envs);
			if (envs.length > 0 && !selectedEnv) {
				setSelectedEnv(envs[0].name);
			}
		} catch (error) {
			setError(
				error instanceof Error ? error.message : "Failed to load environments",
			);
		} finally {
			setIsLoading(false);
		}
	}, [selectedEnv]);

	const loadEnvironmentConfig = useCallback(async (envName: string) => {
		try {
			setIsLoading(true);
			setError(null);
			const content = await infrastructureApi.getEnvironmentConfig(envName);
			setConfigContent(content);
		} catch (error) {
			setError(
				error instanceof Error ? error.message : "Failed to load configuration",
			);
		} finally {
			setIsLoading(false);
		}
	}, []);

	useEffect(() => {
		loadEnvironments();
	}, [loadEnvironments]);

	useEffect(() => {
		if (selectedEnvironment && selectedEnvironment !== selectedEnv) {
			setSelectedEnv(selectedEnvironment);
		}
	}, [selectedEnvironment, selectedEnv]);

	useEffect(() => {
		if (selectedEnv) {
			loadEnvironmentConfig(selectedEnv);
		}
	}, [selectedEnv, loadEnvironmentConfig]);

	const saveConfiguration = async () => {
		if (!selectedEnv) return;

		try {
			setIsSaving(true);
			setError(null);
			setSuccessMessage(null);
			await infrastructureApi.updateEnvironmentConfig(
				selectedEnv,
				configContent,
			);
			setSuccessMessage("Configuration saved successfully!");
			setTimeout(() => setSuccessMessage(null), 3000);
		} catch (error) {
			setError(
				error instanceof Error ? error.message : "Failed to save configuration",
			);
		} finally {
			setIsSaving(false);
		}
	};

	return (
		<Card className="w-full max-w-4xl mx-auto">
			<CardHeader>
				<CardTitle>Environment Configuration</CardTitle>
				<CardDescription>
					Manage your infrastructure environment configurations
				</CardDescription>
			</CardHeader>
			<CardContent>
				<div className="space-y-4">
					<div className="flex items-center gap-4">
						<Select
							value={selectedEnv}
							onValueChange={setSelectedEnv}
							disabled={isLoading}
						>
							<SelectTrigger className="w-[200px]">
								<SelectValue placeholder="Select environment" />
							</SelectTrigger>
							<SelectContent>
								{environments.map((env) => (
									<SelectItem key={env.name} value={env.name}>
										{env.name}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						<Button
							onClick={loadEnvironments}
							variant="outline"
							disabled={isLoading}
						>
							Refresh
						</Button>
					</div>

					{error && (
						<Alert variant="destructive">
							<AlertDescription>{error}</AlertDescription>
						</Alert>
					)}

					{successMessage && (
						<Alert>
							<AlertDescription>{successMessage}</AlertDescription>
						</Alert>
					)}

					{selectedEnv && (
						<Tabs defaultValue="editor" className="w-full">
							<TabsList className="grid w-full grid-cols-2">
								<TabsTrigger value="editor">Editor</TabsTrigger>
								<TabsTrigger value="preview">Preview</TabsTrigger>
							</TabsList>
							<TabsContent value="editor" className="space-y-4">
								<Textarea
									value={configContent}
									onChange={(e) => setConfigContent(e.target.value)}
									className="min-h-[500px] font-mono text-sm"
									placeholder="Loading configuration..."
									disabled={isLoading}
								/>
								<div className="flex justify-end gap-2">
									<Button
										onClick={() => loadEnvironmentConfig(selectedEnv)}
										variant="outline"
										disabled={isLoading}
									>
										Revert Changes
									</Button>
									<Button
										onClick={saveConfiguration}
										disabled={isLoading || isSaving}
									>
										{isSaving ? "Saving..." : "Save Configuration"}
									</Button>
								</div>
							</TabsContent>
							<TabsContent value="preview" className="space-y-4">
								<div className="border rounded-lg p-4 min-h-[500px] bg-gray-50">
									<pre className="text-sm font-mono whitespace-pre-wrap">
										{configContent || "No content to preview"}
									</pre>
								</div>
							</TabsContent>
						</Tabs>
					)}
				</div>
			</CardContent>
		</Card>
	);
}

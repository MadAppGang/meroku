import { FileCode, Info, Package } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { customTerraformApi } from "../api/customTerraform";
import type {
	CustomModuleStatus,
	CustomTerraformFile,
} from "../types/customTerraform";
import { CustomTerraformManager } from "./CustomTerraformManager";
import { Alert, AlertDescription } from "./ui/alert";
import { ScrollArea } from "./ui/scroll-area";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./ui/tabs";

interface CustomTerraformNodePropertiesProps {
	environment: string;
}

export function CustomTerraformNodeProperties({
	environment,
}: CustomTerraformNodePropertiesProps) {
	const [files, setFiles] = useState<CustomTerraformFile[]>([]);
	const [modules, setModules] = useState<CustomModuleStatus[]>([]);
	const [isLoading, setIsLoading] = useState(true);

	const loadData = useCallback(async () => {
		try {
			setIsLoading(true);
			const [filesData, modulesData] = await Promise.all([
				customTerraformApi.listFiles(environment),
				customTerraformApi.getCustomModuleStatus(environment),
			]);
			setFiles(filesData);
			setModules(modulesData.modules);
		} catch (error) {
			console.error("Failed to load custom Terraform data:", error);
		} finally {
			setIsLoading(false);
		}
	}, [environment]);

	// loadData is rebuilt only when environment changes, so this still reloads
	// exactly once per environment.
	useEffect(() => {
		loadData();
	}, [loadData]);

	return (
		<Tabs defaultValue="editor" className="h-full flex flex-col">
			<TabsList className="bg-gray-800 border-b border-gray-700 w-full justify-start rounded-none">
				<TabsTrigger value="editor" className="data-[state=active]:bg-gray-900">
					<FileCode className="w-4 h-4 mr-2" />
					Editor
				</TabsTrigger>
				<TabsTrigger value="files" className="data-[state=active]:bg-gray-900">
					<Info className="w-4 h-4 mr-2" />
					Files
				</TabsTrigger>
				<TabsTrigger
					value="modules"
					className="data-[state=active]:bg-gray-900"
				>
					<Package className="w-4 h-4 mr-2" />
					Modules
				</TabsTrigger>
			</TabsList>

			{/* Editor Tab */}
			<TabsContent value="editor" className="flex-1 m-0">
				<div className="h-full">
					<CustomTerraformManager environment={environment} />
				</div>
			</TabsContent>

			{/* Files Tab */}
			<TabsContent value="files" className="flex-1 m-0">
				<ScrollArea className="h-full">
					<div className="p-4 space-y-4">
						<Alert className="bg-blue-900/20 border-blue-700">
							<Info className="w-4 h-4" />
							<AlertDescription className="text-sm text-gray-300">
								Custom Terraform files are stored in{" "}
								<code className="text-blue-400">custom/terraform/</code>{" "}
								directory. Files in{" "}
								<code className="text-blue-400">_shared/</code> are available to
								all environments.
							</AlertDescription>
						</Alert>

						{isLoading ? (
							<div className="text-center py-8 text-gray-400">
								Loading files...
							</div>
						) : files.length === 0 ? (
							<div className="text-center py-8 text-gray-400">
								<FileCode className="w-12 h-12 mx-auto mb-4 text-gray-600" />
								<p>No custom Terraform files yet</p>
								<p className="text-sm mt-2">Create a new file to get started</p>
							</div>
						) : (
							<div className="space-y-4">
								{/* Shared Files */}
								{files.filter((f) => f.scope === "shared").length > 0 && (
									<div>
										<h3 className="text-sm font-semibold text-white mb-2">
											Shared Files
										</h3>
										<div className="space-y-2">
											{files
												.filter((f) => f.scope === "shared")
												.map((file) => (
													<div
														key={file.path}
														className="bg-gray-800 rounded-lg p-3 border border-gray-700"
													>
														<div className="flex items-start justify-between">
															<div className="flex items-center gap-2">
																<FileCode className="w-4 h-4 text-blue-400" />
																<div>
																	<div className="text-sm font-medium text-white">
																		{file.name}
																	</div>
																	{file.lastModified && (
																		<div className="text-xs text-gray-400">
																			Modified:{" "}
																			{new Date(
																				file.lastModified,
																			).toLocaleString()}
																		</div>
																	)}
																</div>
															</div>
														</div>
													</div>
												))}
										</div>
									</div>
								)}

								{/* Environment Files */}
								{files.filter((f) => f.scope === "environment").length > 0 && (
									<div>
										<h3 className="text-sm font-semibold text-white mb-2">
											Environment Files ({environment})
										</h3>
										<div className="space-y-2">
											{files
												.filter((f) => f.scope === "environment")
												.map((file) => (
													<div
														key={file.path}
														className="bg-gray-800 rounded-lg p-3 border border-gray-700"
													>
														<div className="flex items-start justify-between">
															<div className="flex items-center gap-2">
																<FileCode className="w-4 h-4 text-green-400" />
																<div>
																	<div className="text-sm font-medium text-white">
																		{file.name}
																	</div>
																	{file.lastModified && (
																		<div className="text-xs text-gray-400">
																			Modified:{" "}
																			{new Date(
																				file.lastModified,
																			).toLocaleString()}
																		</div>
																	)}
																</div>
															</div>
														</div>
													</div>
												))}
										</div>
									</div>
								)}
							</div>
						)}
					</div>
				</ScrollArea>
			</TabsContent>

			{/* Modules Tab */}
			<TabsContent value="modules" className="flex-1 m-0">
				<ScrollArea className="h-full">
					<div className="p-4 space-y-4">
						<Alert className="bg-blue-900/20 border-blue-700">
							<Info className="w-4 h-4" />
							<AlertDescription className="text-sm text-gray-300">
								Custom modules deployment status. Run{" "}
								<code className="text-blue-400">terraform apply</code> to deploy
								your changes.
							</AlertDescription>
						</Alert>

						{isLoading ? (
							<div className="text-center py-8 text-gray-400">
								Loading modules...
							</div>
						) : modules.length === 0 ? (
							<div className="text-center py-8 text-gray-400">
								<Package className="w-12 h-12 mx-auto mb-4 text-gray-600" />
								<p>No custom modules deployed</p>
								<p className="text-sm mt-2">
									Create Terraform resources and apply to see them here
								</p>
							</div>
						) : (
							<div className="space-y-3">
								{modules.map((module) => (
									<div
										key={module.moduleName}
										className="bg-gray-800 rounded-lg p-4 border border-gray-700"
									>
										<div className="flex items-start justify-between mb-3">
											<div className="flex items-center gap-2">
												<Package className="w-5 h-5 text-purple-400" />
												<div>
													<div className="text-sm font-medium text-white">
														{module.moduleName}
													</div>
													{module.lastApplied && (
														<div className="text-xs text-gray-400">
															Last applied:{" "}
															{new Date(module.lastApplied).toLocaleString()}
														</div>
													)}
												</div>
											</div>
											<div
												className={`px-2 py-1 rounded text-xs font-medium ${
													module.deployed
														? "bg-green-900/30 text-green-400"
														: "bg-gray-700 text-gray-400"
												}`}
											>
												{module.deployed ? "Deployed" : "Not Deployed"}
											</div>
										</div>

										{module.resourceCount !== undefined && (
											<div className="text-sm text-gray-400 mb-2">
												{module.resourceCount} resource
												{module.resourceCount !== 1 ? "s" : ""}
											</div>
										)}

										{module.outputs && module.outputs.length > 0 && (
											<div className="mt-3 pt-3 border-t border-gray-700">
												<div className="text-xs font-semibold text-gray-400 mb-2">
													Outputs
												</div>
												<div className="space-y-2">
													{module.outputs.map((output) => (
														<div
															key={output.name}
															className="bg-gray-900 rounded p-2"
														>
															<div className="text-xs font-mono text-blue-400">
																{output.name}
															</div>
															<div className="text-xs text-gray-300 mt-1">
																{output.description}
															</div>
															<div className="text-xs font-mono text-gray-500 mt-1">
																{output.value}
															</div>
														</div>
													))}
												</div>
											</div>
										)}
									</div>
								))}
							</div>
						)}
					</div>
				</ScrollArea>
			</TabsContent>
		</Tabs>
	);
}

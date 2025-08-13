import { AlertCircle, ExternalLink, FileText } from "lucide-react";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "./ui/dialog";

interface AmplifyBuildLogsProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	appId: string;
	appName: string;
	branchName: string;
	buildId?: string;
	region?: string;
}

export function AmplifyBuildLogs({
	open,
	onOpenChange,
	appId,
	appName,
	branchName,
	buildId,
	region = "us-east-1",
}: AmplifyBuildLogsProps) {
	const getConsoleUrl = () => {
		if (buildId) {
			return `https://console.aws.amazon.com/amplify/home?region=${region}#/${appId}/${branchName}/${buildId}`;
		}
		return `https://console.aws.amazon.com/amplify/home?region=${region}#/${appId}/${branchName}`;
	};

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-2xl">
				<DialogHeader>
					<DialogTitle className="flex items-center gap-2">
						<FileText className="w-5 h-5" />
						Build Logs
					</DialogTitle>
					<DialogDescription>
						{appName} - {branchName} {buildId && `(Build #${buildId})`}
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4">
					<Alert>
						<AlertCircle className="h-4 w-4" />
						<AlertDescription>
							Build logs are available in the AWS Amplify Console. Click the
							button below to view detailed logs, build artifacts, and
							deployment information.
						</AlertDescription>
					</Alert>

					<div className="bg-muted rounded-lg p-4">
						<h4 className="text-sm font-medium mb-2">Quick Actions</h4>
						<div className="space-y-2">
							<Button
								variant="outline"
								className="w-full justify-start"
								asChild
							>
								<a
									href={getConsoleUrl()}
									target="_blank"
									rel="noopener noreferrer"
								>
									<ExternalLink className="w-4 h-4 mr-2" />
									View Build Logs in AWS Console
								</a>
							</Button>

							<Button
								variant="outline"
								className="w-full justify-start"
								asChild
							>
								<a
									href={`https://console.aws.amazon.com/amplify/home?region=${region}#/${appId}/${branchName}/deployments`}
									target="_blank"
									rel="noopener noreferrer"
								>
									<ExternalLink className="w-4 h-4 mr-2" />
									View Deployment History
								</a>
							</Button>

							<Button
								variant="outline"
								className="w-full justify-start"
								asChild
							>
								<a
									href={`https://console.aws.amazon.com/amplify/home?region=${region}#/${appId}/settings/environment-variables`}
									target="_blank"
									rel="noopener noreferrer"
								>
									<ExternalLink className="w-4 h-4 mr-2" />
									View Environment Variables
								</a>
							</Button>
						</div>
					</div>

					<div className="bg-muted/50 rounded-lg p-4 text-sm text-muted-foreground">
						<p className="mb-2">
							<strong>Note:</strong> Real-time build logs streaming is available
							directly in the AWS Console.
						</p>
						<p>The AWS Amplify Console provides:</p>
						<ul className="list-disc list-inside mt-1 space-y-1">
							<li>Real-time build output and logs</li>
							<li>Build phase breakdown and timing</li>
							<li>Error messages and debugging information</li>
							<li>Artifact downloads and previews</li>
							<li>Performance metrics and analytics</li>
						</ul>
					</div>
				</div>
			</DialogContent>
		</Dialog>
	);
}

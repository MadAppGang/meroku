import { useState } from "react";
import { Copy, Check, GitBranch, Zap } from "lucide-react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";

interface ServiceCICDConfigurationProps {
  config: YamlInfrastructureConfig;
  serviceName: string;
  serviceType?: string;
}

export default function ServiceCICDConfiguration({
  config,
  serviceName,
}: ServiceCICDConfigurationProps) {
  const [copied, setCopied] = useState(false);

  const env = config.env || "dev";
  const region = config.region || "us-east-1";
  const project = config.project || "myproject";
  const accountId = config.account_id || "123456789012";

  // Find service-specific configuration
  const serviceConfig =
    serviceName === "backend"
      ? null // Backend uses global settings
      : config.services?.find((service) => service.name === serviceName);

  // Helper function to detect cross-account ECR
  const isCrossAccountECRUri = (uri: string): boolean => {
    // ECR URI format: {account-id}.dkr.ecr.{region}.amazonaws.com/{repo-name}
    const ecrPattern = /^(\d+)\.dkr\.ecr\.[^.]+\.amazonaws\.com\/.+$/;
    const match = uri.match(ecrPattern);
    if (match) {
      const repoAccountId = match[1];
      return repoAccountId !== accountId; // Different account = cross-account
    }
    return false;
  };

  // Helper function to detect if URI is Docker Hub or other registry
  const isCustomDockerRepo = (uri: string): boolean => {
    // Not an ECR URI = custom repo (Docker Hub, ghcr.io, etc.)
    const ecrPattern = /^\d+\.dkr\.ecr\.[^.]+\.amazonaws\.com\/.+$/;
    return !ecrPattern.test(uri);
  };

  // Determine workflow type based on service configuration
  let isCrossAccountECR = false;
  let isCustomRepo = false;

  if (serviceName === "backend") {
    // Backend uses global ecr_strategy
    isCrossAccountECR = config.ecr_strategy === "cross_account";
  } else if (serviceConfig?.ecr_config) {
    const ecrConfig = serviceConfig.ecr_config;

    if (ecrConfig.mode === "manual_repo" && ecrConfig.repository_uri) {
      // Check if it's cross-account ECR or custom Docker repo
      isCrossAccountECR = isCrossAccountECRUri(ecrConfig.repository_uri);
      isCustomRepo = isCustomDockerRepo(ecrConfig.repository_uri);
    }
    // Note: "use_existing" and "create_ecr" modes use local build-push-deploy workflow
  }

  // Get the custom image URI if it's a custom repo
  const customImageUri = serviceConfig?.ecr_config?.repository_uri || "";

  // Generate the GitHub Actions workflow based on ECR strategy
  const generateWorkflow = () => {
    if (isCrossAccountECR) {
      // Cross-account ECR: EventBridge-only workflow
      return `name: Deploy ${serviceName} to AWS (${env})

on:
  push:
    branches: [main]
    paths:
      - '${serviceName}/**'
      - '.github/workflows/${serviceName}-${env}.yml'
  workflow_dispatch:

concurrency:
  group: deploy-${serviceName}-${env}-\${{ github.ref }}
  cancel-in-progress: true

jobs:
  deploy:
    name: Trigger Deployment via EventBridge
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read

    steps:
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4.0.1
        with:
          role-to-assume: arn:aws:iam::${accountId}:role/${project}-github-actions-${env}
          aws-region: ${region}

      - name: Trigger deployment via EventBridge
        run: |
          echo "Sending deployment event to EventBridge..."
          aws events put-events \\
            --entries '[{
              "Source": "github.actions.${env}",
              "DetailType": "SERVICE_DEPLOY",
              "Detail": "{\\"service\\":\\"${serviceName}\\",\\"env\\":\\"${env}\\",\\"trigger\\":\\"github\\",\\"commit\\":\\"'\${{ github.sha }}'\\",\\"branch\\":\\"'\${{ github.ref_name }}'\\"}",
              "EventBusName": "default"
            }]'
          echo "✅ Deployment event sent successfully"

      - name: Deployment Status
        run: |
          echo "🚀 Deployment triggered for ${serviceName} in ${env} environment"
          echo "📦 The event-driven ECS service will:"
          echo "   1. Pull the latest image from cross-account ECR"
          echo "   2. Update the ECS service"
          echo "   3. Wait for service stability"
          echo ""
          echo "Monitor deployment in AWS Console:"
          echo "https://console.aws.amazon.com/ecs/v2/clusters/${project}-${env}/services/${serviceName}/health?region=${region}"`;
    } else if (isCustomRepo) {
      // Custom Docker Repo: Deploy existing image workflow
      return `name: Deploy ${serviceName} to AWS (${env})

on:
  push:
    branches: [main]
    paths:
      - '${serviceName}/**'
      - '.github/workflows/${serviceName}-${env}.yml'
  workflow_dispatch:

concurrency:
  group: deploy-${serviceName}-${env}-\${{ github.ref }}
  cancel-in-progress: true

jobs:
  deploy:
    name: Deploy Custom Docker Image to ECS
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read

    steps:
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4.0.1
        with:
          role-to-assume: arn:aws:iam::${accountId}:role/${project}-github-actions-${env}
          aws-region: ${region}

      - name: Deploy to ECS
        run: |
          echo "Deploying ${serviceName} with image: ${customImageUri}"
          aws ecs update-service \\
            --cluster ${project}-${env} \\
            --service ${serviceName} \\
            --force-new-deployment \\
            --region ${region}

      - name: Wait for service stability
        run: |
          echo "Waiting for service to stabilize..."
          aws ecs wait services-stable \\
            --cluster ${project}-${env} \\
            --services ${serviceName} \\
            --region ${region}
          echo "✅ Deployment completed successfully"
          echo "Using Docker image: ${customImageUri}"`;
    } else {
      // Local ECR: Full build-push-deploy workflow
      return `name: Deploy ${serviceName} to AWS (${env})

on:
  push:
    branches: [main]
    paths:
      - '${serviceName}/**'
      - '.github/workflows/${serviceName}-${env}.yml'
  workflow_dispatch:

concurrency:
  group: deploy-${serviceName}-${env}-\${{ github.ref }}
  cancel-in-progress: true

jobs:
  deploy:
    name: Build and Deploy to ECS
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4.0.1
        with:
          role-to-assume: arn:aws:iam::${accountId}:role/${project}-github-actions-${env}
          aws-region: ${region}

      - name: Login to Amazon ECR
        id: login-ecr
        uses: aws-actions/amazon-ecr-login@v2

      - name: Build, tag, and push Docker image
        env:
          ECR_REGISTRY: \${{ steps.login-ecr.outputs.registry }}
          ECR_REPOSITORY: ${serviceName}
          IMAGE_TAG: \${{ github.sha }}
        run: |
          # Build Docker image
          docker build -t \$ECR_REGISTRY/\$ECR_REPOSITORY:\$IMAGE_TAG ${serviceName}/
          docker tag \$ECR_REGISTRY/\$ECR_REPOSITORY:\$IMAGE_TAG \$ECR_REGISTRY/\$ECR_REPOSITORY:latest

          # Create ECR repository if it doesn't exist
          aws ecr describe-repositories --repository-names \$ECR_REPOSITORY || \\
            aws ecr create-repository --repository-name \$ECR_REPOSITORY

          # Push images
          docker push \$ECR_REGISTRY/\$ECR_REPOSITORY:\$IMAGE_TAG
          docker push \$ECR_REGISTRY/\$ECR_REPOSITORY:latest

          echo "✅ Image pushed: \$ECR_REGISTRY/\$ECR_REPOSITORY:\$IMAGE_TAG"

      - name: Deploy to ECS
        run: |
          echo "Deploying ${serviceName} to ECS..."
          aws ecs update-service \\
            --cluster ${project}-${env} \\
            --service ${serviceName} \\
            --force-new-deployment \\
            --region ${region}

      - name: Wait for service stability
        run: |
          echo "Waiting for service to stabilize..."
          aws ecs wait services-stable \\
            --cluster ${project}-${env} \\
            --services ${serviceName} \\
            --region ${region}
          echo "✅ Deployment completed successfully"`;
    }
  };

  const workflow = generateWorkflow();

  const handleCopy = () => {
    navigator.clipboard.writeText(workflow);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-6">
      {/* Strategy Badge */}
      <div className="flex items-center gap-3">
        <div
          className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium ${
            isCrossAccountECR
              ? "bg-purple-500/10 text-purple-400 border border-purple-500/20"
              : isCustomRepo
                ? "bg-green-500/10 text-green-400 border border-green-500/20"
                : "bg-blue-500/10 text-blue-400 border border-blue-500/20"
          }`}
        >
          {isCrossAccountECR ? (
            <>
              <Zap className="w-4 h-4" />
              Cross-Account ECR (EventBridge)
            </>
          ) : isCustomRepo ? (
            <>
              <Zap className="w-4 h-4" />
              Custom Docker Image (Docker Hub, etc.)
            </>
          ) : (
            <>
              <GitBranch className="w-4 h-4" />
              Local ECR (Build & Push)
            </>
          )}
        </div>
      </div>

      {/* Workflow File */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-slate-200">
            Workflow File
          </h3>
          <button
            onClick={handleCopy}
            className="flex items-center gap-2 px-3 py-1.5 bg-slate-700 hover:bg-slate-600 border border-slate-600 rounded-lg text-sm text-slate-200 transition-colors"
          >
            {copied ? (
              <>
                <Check className="w-4 h-4 text-green-400" />
                Copied!
              </>
            ) : (
              <>
                <Copy className="w-4 h-4" />
                Copy Workflow
              </>
            )}
          </button>
        </div>

        <div className="bg-slate-900 border border-slate-700 rounded-lg p-4 overflow-x-auto">
          <div className="text-xs text-slate-500 mb-2 font-mono">
            .github/workflows/{serviceName}-{env}.yml
          </div>
          <pre className="text-sm text-slate-300 font-mono whitespace-pre">
            {workflow}
          </pre>
        </div>
      </div>

      {/* Additional Resources */}
      {isCrossAccountECR && (
        <div className="bg-slate-800/50 border border-slate-700 rounded-lg p-4">
          <h3 className="text-sm font-semibold text-slate-200 mb-3">
            EventBridge Event Schema
          </h3>
          <pre className="text-xs text-slate-300 font-mono bg-slate-900 rounded p-3 overflow-x-auto">
            {JSON.stringify(
              {
                Source: `github.actions.${env}`,
                DetailType: "SERVICE_DEPLOY",
                Detail: {
                  service: serviceName,
                  env: env,
                  trigger: "github",
                  commit: "<github.sha>",
                  branch: "<github.ref_name>",
                },
              },
              null,
              2
            )}
          </pre>
          <div className="mt-3 space-y-2">
            <p className="text-xs text-slate-400">
              This event will be matched by EventBridge rules configured to listen
              for <code className="px-1 py-0.5 bg-slate-800 rounded">github.actions.{env}</code> source
              and <code className="px-1 py-0.5 bg-slate-800 rounded">SERVICE_DEPLOY</code> detail type.
            </p>
            <p className="text-xs text-slate-400">
              <strong className="text-slate-300">Deployment Flow:</strong> EventBridge → Lambda Function → ECS Service Update
            </p>
            <p className="text-xs text-slate-400">
              The deployment Lambda function will receive this event, pull the latest image from the cross-account ECR,
              and trigger an ECS service update with the new image. ECS redeployment typically takes <strong className="text-slate-300">1-3 minutes</strong>.
            </p>
            <p className="text-xs text-slate-400">
              💡 <strong className="text-slate-300">Tip:</strong> You can configure the Lambda function with a Slack webhook URL
              to receive deployment notifications in your Slack channel (start, success, or failure status).
            </p>
          </div>
        </div>
      )}

      {isCustomRepo && (
        <div className="bg-slate-800/50 border border-slate-700 rounded-lg p-4">
          <h3 className="text-sm font-semibold text-slate-200 mb-3">
            Custom Docker Image Configuration
          </h3>
          <div className="space-y-3">
            <div className="bg-slate-900 rounded p-3">
              <p className="text-xs text-slate-500 mb-1">Docker Image URI</p>
              <code className="text-xs text-slate-300 font-mono break-all">
                {customImageUri}
              </code>
            </div>
            <div className="space-y-2">
              <p className="text-xs text-slate-400">
                <strong className="text-slate-300">Deployment Flow:</strong> GitHub Actions → AWS ECS Service Update
              </p>
              <p className="text-xs text-slate-400">
                This workflow deploys the pre-built Docker image from Docker Hub, GHCR, or any other container registry.
                The workflow does not build a new image; it triggers a deployment with the existing image configured in the service.
                ECS redeployment typically takes <strong className="text-slate-300">1-3 minutes</strong>.
              </p>
              <p className="text-xs text-slate-400">
                💡 <strong className="text-slate-300">Tip:</strong> Make sure the ECS task definition references the correct image URI.
                Update the image tag in your service configuration when you want to deploy a different version.
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

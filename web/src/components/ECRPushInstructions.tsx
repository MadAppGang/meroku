import { useState } from 'react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { type AccountInfo } from '../api/infrastructure';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Copy, Check, Terminal, Info } from 'lucide-react';

interface ECRPushInstructionsProps {
  config: YamlInfrastructureConfig;
  accountInfo?: AccountInfo;
}

export function ECRPushInstructions({ config, accountInfo }: ECRPushInstructionsProps) {
  const [copiedCommand, setCopiedCommand] = useState<string | null>(null);

  const handleCopyCommand = (command: string, id: string) => {
    navigator.clipboard.writeText(command);
    setCopiedCommand(id);
    setTimeout(() => setCopiedCommand(null), 2000);
  };

  const isUsingCrossAccount = config.ecr_account_id ? true : false;
  const accountId = isUsingCrossAccount ? config.ecr_account_id : accountInfo?.accountId;
  const region = isUsingCrossAccount && config.ecr_account_region ? config.ecr_account_region : config.region;
  const repositoryName = `${config.project}_backend`; // Example repository

  if (!accountId) {
    return (
      <div className="flex items-center justify-center py-8">
        <p className="text-sm text-gray-400">Loading account information...</p>
      </div>
    );
  }

  const ecrUri = `${accountId}.dkr.ecr.${region}.amazonaws.com`;
  const fullRepositoryUri = `${ecrUri}/${repositoryName}`;

  const commands = {
    login: `aws ecr get-login-password --region ${region} | docker login --username AWS --password-stdin ${ecrUri}`,
    build: `docker build -t ${repositoryName} .`,
    tag: `docker tag ${repositoryName}:latest ${fullRepositoryUri}:latest`,
    push: `docker push ${fullRepositoryUri}:latest`
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Push to ECR</CardTitle>
          <CardDescription>
            Step-by-step instructions to push Docker images to your ECR repository
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* Prerequisites */}
          <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-blue-400 mt-0.5" />
              <div className="flex-1">
                <h4 className="text-sm font-medium text-blue-400 mb-2">Prerequisites</h4>
                <ul className="text-xs text-gray-300 space-y-1">
                  <li>• AWS CLI installed and configured with appropriate credentials</li>
                  <li>• Docker installed and running</li>
                  <li>• ECR repository created (happens automatically in dev environment)</li>
                  {isUsingCrossAccount && (
                    <li>• Cross-account permissions configured for ECR access</li>
                  )}
                </ul>
              </div>
            </div>
          </div>

          {/* Step 1: Login */}
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">1</div>
              <h3 className="text-sm font-medium">Authenticate Docker to ECR</h3>
            </div>
            <div className="ml-8">
              <p className="text-xs text-gray-400 mb-2">Login to ECR using AWS CLI:</p>
              <div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
                <Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
                <code className="text-xs text-gray-300 flex-1 break-all">{commands.login}</code>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-6 w-6"
                  onClick={() => handleCopyCommand(commands.login, 'login')}
                >
                  {copiedCommand === 'login' ? (
                    <Check className="h-3 w-3 text-green-400" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>

          {/* Step 2: Build */}
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">2</div>
              <h3 className="text-sm font-medium">Build Docker Image</h3>
            </div>
            <div className="ml-8">
              <p className="text-xs text-gray-400 mb-2">Build your Docker image (run from your project directory):</p>
              <div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
                <Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
                <code className="text-xs text-gray-300 flex-1">{commands.build}</code>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-6 w-6"
                  onClick={() => handleCopyCommand(commands.build, 'build')}
                >
                  {copiedCommand === 'build' ? (
                    <Check className="h-3 w-3 text-green-400" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>

          {/* Step 3: Tag */}
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">3</div>
              <h3 className="text-sm font-medium">Tag Image for ECR</h3>
            </div>
            <div className="ml-8">
              <p className="text-xs text-gray-400 mb-2">Tag your image with the ECR repository URI:</p>
              <div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
                <Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
                <code className="text-xs text-gray-300 flex-1 break-all">{commands.tag}</code>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-6 w-6"
                  onClick={() => handleCopyCommand(commands.tag, 'tag')}
                >
                  {copiedCommand === 'tag' ? (
                    <Check className="h-3 w-3 text-green-400" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>

          {/* Step 4: Push */}
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">4</div>
              <h3 className="text-sm font-medium">Push to ECR</h3>
            </div>
            <div className="ml-8">
              <p className="text-xs text-gray-400 mb-2">Push your image to ECR:</p>
              <div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
                <Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
                <code className="text-xs text-gray-300 flex-1 break-all">{commands.push}</code>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-6 w-6"
                  onClick={() => handleCopyCommand(commands.push, 'push')}
                >
                  {copiedCommand === 'push' ? (
                    <Check className="h-3 w-3 text-green-400" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>

          {/* All-in-one script */}
          <div className="mt-6 space-y-2">
            <h3 className="text-sm font-medium text-gray-300">All-in-One Script</h3>
            <p className="text-xs text-gray-400">Run all commands in sequence:</p>
            <div className="bg-gray-800 rounded-lg p-3">
              <div className="flex items-start gap-2 mb-2">
                <Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
                <code className="text-xs text-gray-300 flex-1 whitespace-pre-wrap break-all">{`#!/bin/bash
# Login to ECR
${commands.login}

# Build the image
${commands.build}

# Tag the image
${commands.tag}

# Push to ECR
${commands.push}`}</code>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-6 w-6"
                  onClick={() => handleCopyCommand(`#!/bin/bash\n# Login to ECR\n${commands.login}\n\n# Build the image\n${commands.build}\n\n# Tag the image\n${commands.tag}\n\n# Push to ECR\n${commands.push}`, 'script')}
                >
                  {copiedCommand === 'script' ? (
                    <Check className="h-3 w-3 text-green-400" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>

          {/* Additional tips */}
          <div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
            <h4 className="text-sm font-medium text-yellow-400 mb-2">Tips</h4>
            <ul className="text-xs text-gray-300 space-y-1">
              <li>• You can tag images with version numbers: <code className="text-yellow-300">:v1.0.0</code> instead of <code className="text-yellow-300">:latest</code></li>
              <li>• Use <code className="text-yellow-300">docker images</code> to see all local images</li>
              <li>• Use <code className="text-yellow-300">aws ecr describe-images --repository-name {repositoryName}</code> to list pushed images</li>
              <li>• ECR automatically scans images for vulnerabilities if enabled</li>
            </ul>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
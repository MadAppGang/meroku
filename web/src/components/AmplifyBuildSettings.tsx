import { useState } from 'react';
import { Info, Copy, Check } from 'lucide-react';
import type { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Button } from './ui/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from './ui/tabs';

interface AmplifyBuildSettingsProps {
  config: YamlInfrastructureConfig;
  nodeId: string;
  onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
  selectedBranch?: string;
}

export function AmplifyBuildSettings({ config, nodeId, selectedBranch }: AmplifyBuildSettingsProps) {
  const appName = nodeId.replace('amplify-', '');
  const amplifyAppIndex = config.amplify_apps?.findIndex(app => app.name === appName) ?? -1;
  const amplifyApp = config.amplify_apps?.[amplifyAppIndex];
  const [copiedExample, setCopiedExample] = useState<string | null>(null);

  if (!amplifyApp) {
    return (
      <div className="text-gray-400">
        <p>Amplify app configuration not found.</p>
      </div>
    );
  }


  const copyToClipboard = (text: string, exampleName: string) => {
    navigator.clipboard.writeText(text);
    setCopiedExample(exampleName);
    setTimeout(() => setCopiedExample(null), 2000);
  };

  const amplifyYmlExamples = {
    react: `version: 1
frontend:
  phases:
    preBuild:
      commands:
        - npm ci
    build:
      commands:
        - npm run build
  artifacts:
    baseDirectory: build
    files:
      - '**/*'
  cache:
    paths:
      - node_modules/**/*`,
      
    nextjs: `version: 1
frontend:
  phases:
    preBuild:
      commands:
        - npm ci
    build:
      commands:
        - npm run build
  artifacts:
    baseDirectory: .next
    files:
      - '**/*'
  cache:
    paths:
      - node_modules/**/*`,

    monorepo: `version: 1
applications:
  - appRoot: apps/web
    frontend:
      phases:
        preBuild:
          commands:
            - npm ci --prefix ../..
            - npx lerna run build --scope=shared-ui
        build:
          commands:
            - npm run build
      artifacts:
        baseDirectory: dist
        files:
          - '**/*'
      cache:
        paths:
          - ../../node_modules/**/*`,

    custom: `version: 1
frontend:
  phases:
    preBuild:
      commands:
        - echo "Installing dependencies..."
        - yarn install --frozen-lockfile
        - echo "Running custom setup..."
        - npm run setup:env
    build:
      commands:
        - echo "Building application..."
        - npm run build:production
        - echo "Running post-build tasks..."
        - npm run postbuild
  artifacts:
    baseDirectory: dist
    files:
      - '**/*'
    excludeFiles:
      - '**/*.map'
      - '**/node_modules/**/*'
  cache:
    paths:
      - node_modules/**/*
      - .yarn/cache/**/*
  customHeaders:
    - pattern: '**/*'
      headers:
        - key: 'Cache-Control'
          value: 'public, max-age=31536000, immutable'
    - pattern: '**/*.html'
      headers:
        - key: 'Cache-Control'
          value: 'public, max-age=0, must-revalidate'`
  };

  // Get branch info if available
  const branches = amplifyApp?.branches || [];
  const currentBranch = branches.find(b => b.name === selectedBranch) || branches[0];

  return (
    <div className="space-y-6">

      {/* Build Information Header */}
      <div>
        <h3 className="text-sm font-medium text-white mb-4">
          Build Configuration
          {currentBranch && (
            <span className="text-xs text-gray-400 ml-2">
              (Branch: {currentBranch.name})
            </span>
          )}
        </h3>
      </div>

      {/* Auto-Detection Information */}
      <div>
        <h3 className="text-sm font-medium text-white mb-4">How Amplify Build Works</h3>
        <div className="bg-gray-800 rounded-lg p-4 border border-gray-700 space-y-3">
          <div className="flex items-start gap-2">
            <Info className="w-4 h-4 text-blue-400 mt-0.5 flex-shrink-0" />
            <div className="text-sm text-gray-300 space-y-2">
              <p className="font-medium">Amplify automatically detects your framework and build settings:</p>
              <ul className="list-disc list-inside space-y-1 text-xs text-gray-400 ml-2">
                <li>Scans for framework files (package.json, next.config.js, etc.)</li>
                <li>Detects package manager (npm, yarn, pnpm) from lock files</li>
                <li>Identifies framework from dependencies</li>
                <li>Sets appropriate build commands and output directories</li>
              </ul>
            </div>
          </div>

          <div className="border-t border-gray-700 pt-3">
            <p className="text-xs text-gray-400">
              <span className="font-medium">Detected frameworks include:</span> React, Vue, Angular, Next.js, Nuxt.js, 
              Gatsby, Svelte/SvelteKit, Astro, Ember, and more.
            </p>
          </div>
        </div>
      </div>

      {/* Custom Build Configuration */}
      <div>
        <h3 className="text-sm font-medium text-white mb-4">Custom Build Configuration (Optional)</h3>
        <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
          <p className="text-sm text-gray-300 mb-3">
            For custom build requirements, create an <code className="bg-gray-900 px-1 py-0.5 rounded text-blue-400">amplify.yml</code> file 
            in your repository root.
          </p>
          
          <Tabs defaultValue="react" className="mt-4">
            <TabsList className="grid grid-cols-4 w-full bg-gray-900">
              <TabsTrigger value="react" className="text-xs">React/Vue</TabsTrigger>
              <TabsTrigger value="nextjs" className="text-xs">Next.js/Nuxt</TabsTrigger>
              <TabsTrigger value="monorepo" className="text-xs">Monorepo</TabsTrigger>
              <TabsTrigger value="custom" className="text-xs">Custom</TabsTrigger>
            </TabsList>

            <TabsContent value="react" className="mt-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <p className="text-xs text-gray-400">Standard React/Vue build configuration</p>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => copyToClipboard(amplifyYmlExamples.react, 'react')}
                  >
                    {copiedExample === 'react' ? (
                      <>
                        <Check className="w-3 h-3 mr-1" />
                        Copied
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3 mr-1" />
                        Copy
                      </>
                    )}
                  </Button>
                </div>
                <pre className="text-xs text-gray-300 font-mono bg-gray-900 p-3 rounded overflow-x-auto">
{amplifyYmlExamples.react}
                </pre>
              </div>
            </TabsContent>

            <TabsContent value="nextjs" className="mt-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <p className="text-xs text-gray-400">Next.js/Nuxt.js configuration</p>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => copyToClipboard(amplifyYmlExamples.nextjs, 'nextjs')}
                  >
                    {copiedExample === 'nextjs' ? (
                      <>
                        <Check className="w-3 h-3 mr-1" />
                        Copied
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3 mr-1" />
                        Copy
                      </>
                    )}
                  </Button>
                </div>
                <pre className="text-xs text-gray-300 font-mono bg-gray-900 p-3 rounded overflow-x-auto">
{amplifyYmlExamples.nextjs}
                </pre>
              </div>
            </TabsContent>

            <TabsContent value="monorepo" className="mt-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <p className="text-xs text-gray-400">Monorepo with shared dependencies</p>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => copyToClipboard(amplifyYmlExamples.monorepo, 'monorepo')}
                  >
                    {copiedExample === 'monorepo' ? (
                      <>
                        <Check className="w-3 h-3 mr-1" />
                        Copied
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3 mr-1" />
                        Copy
                      </>
                    )}
                  </Button>
                </div>
                <pre className="text-xs text-gray-300 font-mono bg-gray-900 p-3 rounded overflow-x-auto">
{amplifyYmlExamples.monorepo}
                </pre>
              </div>
            </TabsContent>

            <TabsContent value="custom" className="mt-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <p className="text-xs text-gray-400">Advanced configuration with custom headers</p>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => copyToClipboard(amplifyYmlExamples.custom, 'custom')}
                  >
                    {copiedExample === 'custom' ? (
                      <>
                        <Check className="w-3 h-3 mr-1" />
                        Copied
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3 mr-1" />
                        Copy
                      </>
                    )}
                  </Button>
                </div>
                <pre className="text-xs text-gray-300 font-mono bg-gray-900 p-3 rounded overflow-x-auto">
{amplifyYmlExamples.custom}
                </pre>
              </div>
            </TabsContent>
          </Tabs>
        </div>
      </div>

      {/* Additional Build Features */}
      <div>
        <h3 className="text-sm font-medium text-white mb-4">Build Features</h3>
        <div className="bg-gray-800 rounded-lg p-4 border border-gray-700 space-y-3">
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <p className="text-gray-400 text-xs mb-1">Build Image</p>
              <p className="text-gray-300">Amazon Linux 2</p>
            </div>
            <div>
              <p className="text-gray-400 text-xs mb-1">Node.js Versions</p>
              <p className="text-gray-300">12, 14, 16, 18, 20</p>
            </div>
            <div>
              <p className="text-gray-400 text-xs mb-1">Build Timeout</p>
              <p className="text-gray-300">30 minutes (default)</p>
            </div>
            <div>
              <p className="text-gray-400 text-xs mb-1">Build Compute</p>
              <p className="text-gray-300">4 vCPUs, 7 GB memory</p>
            </div>
          </div>
          
          <div className="border-t border-gray-700 pt-3">
            <p className="text-xs text-gray-400">
              <span className="font-medium">Available tools:</span> AWS CLI, Git, cURL, Python, Ruby, and more pre-installed in the build environment.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
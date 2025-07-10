import { GitBranch, ArrowRight, Key } from 'lucide-react';
import type { YamlInfrastructureConfig } from '../types/yamlConfig';

interface AmplifyEnvironmentVariablesProps {
  config: YamlInfrastructureConfig;
  nodeId: string;
  onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
  selectedBranch?: string;
}

export function AmplifyEnvironmentVariables({ config, nodeId }: AmplifyEnvironmentVariablesProps) {
  const appName = nodeId.replace('amplify-', '');
  const amplifyAppIndex = config.amplify_apps?.findIndex(app => app.name === appName) ?? -1;
  const amplifyApp = config.amplify_apps?.[amplifyAppIndex];

  if (!amplifyApp) {
    return (
      <div className="text-gray-400">
        <p>Amplify app configuration not found.</p>
      </div>
    );
  }

  // Get branch statistics
  const branches = amplifyApp.branches || [];
  const totalEnvVars = branches.reduce((sum, branch) => {
    return sum + Object.keys(branch.environment_variables || {}).length;
  }, 0);

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-white mb-4">Environment Variables</h3>
        
        <div className="bg-gray-800 rounded-lg p-6 border border-gray-700 text-center">
          <GitBranch className="w-12 h-12 text-gray-600 mx-auto mb-4" />
          <h4 className="text-lg font-medium text-white mb-2">
            Environment Variables are Branch-Specific
          </h4>
          <p className="text-sm text-gray-400 mb-4">
            Each branch can have its own set of environment variables. 
            Manage them in the Branches tab for better organization.
          </p>
          
          <div className="bg-gray-900 rounded p-4 mb-4">
            <div className="text-sm text-gray-300">
              <p className="font-medium">Current Configuration:</p>
              <div className="mt-2 space-y-1">
                <p>{branches.length} branch{branches.length !== 1 ? 'es' : ''} configured</p>
                <p>{totalEnvVars} total environment variable{totalEnvVars !== 1 ? 's' : ''}</p>
              </div>
            </div>
          </div>

          <div className="flex items-center justify-center gap-2 text-blue-400">
            <span className="text-sm">Go to Branches tab to manage environment variables</span>
            <ArrowRight className="w-4 h-4" />
          </div>
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-white mb-4">Environment Variable Overview</h3>
        <div className="space-y-3">
          {branches.map(branch => {
            const envCount = Object.keys(branch.environment_variables || {}).length;
            return (
              <div key={branch.name} className="bg-gray-800 rounded-lg p-4 border border-gray-700">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <GitBranch className="w-4 h-4 text-gray-400" />
                    <span className="text-sm font-medium text-white">{branch.name}</span>
                    <span className={`px-2 py-0.5 text-xs font-medium rounded-full ${
                      branch.stage === 'PRODUCTION' ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200' :
                      branch.stage === 'BETA' ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200' :
                      branch.stage === 'DEVELOPMENT' ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200' :
                      'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200'
                    }`}>
                      {branch.stage || 'DEVELOPMENT'}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 text-sm text-gray-400">
                    <Key className="w-4 h-4" />
                    <span>{envCount} variable{envCount !== 1 ? 's' : ''}</span>
                  </div>
                </div>
                {envCount > 0 && (
                  <div className="mt-2 text-xs text-gray-400">
                    {Object.keys(branch.environment_variables || {}).slice(0, 3).join(', ')}
                    {Object.keys(branch.environment_variables || {}).length > 3 && '...'}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-white mb-4">Variable Interpolation</h3>
        <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
          <p className="text-sm text-gray-300 mb-3">
            Variables can reference other values using the <code className="text-blue-400">${'{variable}'}</code> syntax.
          </p>
          <div className="space-y-2 text-xs text-gray-400">
            <p className="font-medium">Common interpolated variables:</p>
            <ul className="list-disc list-inside space-y-1 ml-2">
              <li><code className="text-gray-300">PROJECT</code> - Project name</li>
              <li><code className="text-gray-300">ENV</code> - Environment name</li>
              <li><code className="text-gray-300">REGION</code> - AWS region</li>
              <li><code className="text-gray-300">ACCOUNT_ID</code> - AWS account ID</li>
              <li><code className="text-gray-300">cognito_user_pool_id</code> - Cognito user pool ID</li>
              <li><code className="text-gray-300">cognito_web_client_id</code> - Cognito web client ID</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
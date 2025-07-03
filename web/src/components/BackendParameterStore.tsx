import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Key, Lock, FolderOpen, FileText } from 'lucide-react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';

interface BackendParameterStoreProps {
  config: YamlInfrastructureConfig;
}

export function BackendParameterStore({ config }: BackendParameterStoreProps) {
  const parameterPath = `/${config.env}/${config.project}/backend`;

  // Example parameters that would be auto-discovered
  const parameters = [
    {
      name: 'env',
      path: `${parameterPath}/env`,
      type: 'SecureString',
      description: 'Main environment configuration placeholder',
      value: '(empty by default)',
      autoCreated: true,
    },
    {
      name: 'pg_database_password',
      path: `${parameterPath}/pg_database_password`,
      type: 'SecureString',
      description: 'PostgreSQL database password',
      value: '****',
      autoCreated: true,
    },
  ];

  if (config.workload?.setup_fcnsns) {
    parameters.push({
      name: 'gcm-server-key',
      path: `${parameterPath}/gcm-server-key`,
      type: 'SecureString',
      description: 'Google Cloud Messaging server key for push notifications',
      value: '****',
      autoCreated: true,
    });
  }

  // Example of manually added parameters
  const manualParameters = [
    {
      name: 'stripe_api_key',
      path: `${parameterPath}/stripe_api_key`,
      type: 'SecureString',
      description: 'Stripe API key (manually added)',
      value: '****',
      autoCreated: false,
    },
    {
      name: 'jwt_secret',
      path: `${parameterPath}/jwt_secret`,
      type: 'SecureString',
      description: 'JWT signing secret (manually added)',
      value: '****',
      autoCreated: false,
    },
  ];

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <FolderOpen className="w-5 h-5" />
            Parameter Namespace
          </CardTitle>
          <CardDescription>
            <code className="font-mono text-sm">{parameterPath}/*</code>
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="p-3 bg-gray-800 rounded">
            <p className="text-sm text-gray-300 mb-2">All parameters in this namespace are automatically:</p>
            <ul className="text-sm text-gray-400 space-y-1 ml-4">
              <li>• Discovered at runtime using <code className="font-mono text-xs">aws_ssm_parameters_by_path</code></li>
              <li>• Transformed to uppercase environment variables</li>
              <li>• Injected into the ECS container as secrets</li>
              <li>• Encrypted using AWS KMS</li>
            </ul>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Auto-Created Parameters</CardTitle>
          <CardDescription>Parameters created by Terraform</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {parameters.map((param) => (
              <div key={param.path} className="border border-gray-700 rounded p-3 space-y-2">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2">
                    <Key className="w-4 h-4 text-green-400" />
                    <code className="text-sm font-mono text-green-400">{param.name.toUpperCase()}</code>
                  </div>
                  <div className="flex items-center gap-2">
                    <Lock className="w-3 h-3 text-gray-400" />
                    <Badge variant="secondary" className="text-xs">{param.type}</Badge>
                  </div>
                </div>
                <p className="text-xs text-gray-400">{param.description}</p>
                <div className="flex items-center gap-2 text-xs">
                  <FileText className="w-3 h-3 text-gray-500" />
                  <code className="text-gray-500">{param.path}</code>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Manually Added Parameters</CardTitle>
          <CardDescription>Example parameters that can be added via AWS Console or CLI</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {manualParameters.map((param) => (
              <div key={param.path} className="border border-gray-700 rounded p-3 space-y-2 opacity-60">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2">
                    <Key className="w-4 h-4 text-blue-400" />
                    <code className="text-sm font-mono text-blue-400">{param.name.toUpperCase()}</code>
                  </div>
                  <div className="flex items-center gap-2">
                    <Lock className="w-3 h-3 text-gray-400" />
                    <Badge variant="outline" className="text-xs">Manual</Badge>
                  </div>
                </div>
                <p className="text-xs text-gray-400">{param.description}</p>
                <div className="flex items-center gap-2 text-xs">
                  <FileText className="w-3 h-3 text-gray-500" />
                  <code className="text-gray-500">{param.path}</code>
                </div>
              </div>
            ))}
          </div>
          <div className="mt-4 p-3 bg-blue-900/20 border border-blue-700 rounded">
            <p className="text-xs text-blue-300">
              💡 Add new parameters using: <code className="font-mono">aws ssm put-parameter --name "{parameterPath}/your_key" --value "your_value" --type SecureString</code>
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
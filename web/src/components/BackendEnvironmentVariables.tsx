import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Separator } from './ui/separator';
import { YamlInfrastructureConfig } from '../types/yamlConfig';

interface BackendEnvironmentVariablesProps {
  config: YamlInfrastructureConfig;
}

export function BackendEnvironmentVariables({ config }: BackendEnvironmentVariablesProps) {
  // Static environment variables that are always set
  const staticEnvVars = [
    { name: 'PORT', value: config.workload?.backend_image_port?.toString() || '8080', source: 'static' },
    { name: 'PG_DATABASE_HOST', value: 'RDS Endpoint', source: 'static' },
    { name: 'PG_DATABASE_USERNAME', value: 'postgres', source: 'static' },
    { name: 'PG_DATABASE_NAME', value: config.postgres?.db_name || 'database', source: 'static' },
    { name: 'AWS_S3_BUCKET', value: `${config.project}-backend-${config.env}-${config.workload?.bucket_postfix || ''}`, source: 'static' },
    { name: 'AWS_REGION', value: 'Current Region', source: 'static' },
    { name: 'URL', value: 'API Domain', source: 'static' },
    { name: 'SQS_QUEUE_URL', value: 'SQS Queue URL', source: 'static' },
    { name: 'AWS_QUEUE_URL', value: 'SQS Queue URL', source: 'static' },
  ];

  // Custom environment variables from config
  const customEnvVars = config.workload?.backend_env_variables || [];

  // SSM injected variables (examples)
  const ssmEnvVars = [
    { name: 'PG_DATABASE_PASSWORD', value: '****', source: 'ssm', path: `/${config.env}/${config.project}/backend/pg_database_password` },
    { name: 'ENV', value: '(from SSM)', source: 'ssm', path: `/${config.env}/${config.project}/backend/env` },
  ];

  if (config.workload?.setup_fcnsns) {
    ssmEnvVars.push({
      name: 'GCM_SERVER_KEY',
      value: '****',
      source: 'ssm',
      path: `/${config.env}/${config.project}/backend/gcm-server-key`
    });
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Static Environment Variables</CardTitle>
          <CardDescription>Environment variables defined in Terraform configuration</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {staticEnvVars.map((envVar) => (
              <div key={envVar.name} className="flex items-center justify-between p-2 bg-gray-800 rounded">
                <code className="text-sm font-mono text-blue-400">{envVar.name}</code>
                <span className="text-sm text-gray-300 font-mono">{envVar.value}</span>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {customEnvVars.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Custom Environment Variables</CardTitle>
            <CardDescription>Variables from YAML configuration</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {customEnvVars.map((envVar, index) => (
                <div key={index} className="flex items-center justify-between p-2 bg-gray-800 rounded">
                  <code className="text-sm font-mono text-blue-400">{envVar.name}</code>
                  <span className="text-sm text-gray-300 font-mono">{envVar.value}</span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>SSM Parameter Store Variables</CardTitle>
          <CardDescription>Auto-discovered from Parameter Store namespace</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {ssmEnvVars.map((envVar) => (
              <div key={envVar.name} className="space-y-1">
                <div className="flex items-center justify-between p-2 bg-gray-800 rounded">
                  <code className="text-sm font-mono text-green-400">{envVar.name}</code>
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-gray-400 font-mono">{envVar.value}</span>
                    <Badge variant="outline" className="text-xs">SSM</Badge>
                  </div>
                </div>
                <p className="text-xs text-gray-500 ml-2">{envVar.path}</p>
              </div>
            ))}
          </div>
          <div className="mt-4 p-3 bg-blue-900/20 border border-blue-700 rounded">
            <p className="text-xs text-blue-300">
              💡 All parameters under <code className="font-mono">/{config.env}/{config.project}/backend/</code> are automatically injected as environment variables
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
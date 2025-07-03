import React from 'react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Key, Server, Package, Clock, Info } from 'lucide-react';

interface ParameterStoreNodePropertiesProps {
  config: YamlInfrastructureConfig;
}

export function ParameterStoreNodeProperties({ config }: ParameterStoreNodePropertiesProps) {
  // Generate list of parameters based on services
  const parameters = [];
  
  // Backend service parameters - always created
  parameters.push({
    name: `/${config.env}/${config.project}/backend/env`,
    description: 'Main environment configuration placeholder',
    type: 'SecureString',
    icon: Server,
    color: 'text-blue-400',
    note: 'Initially blank'
  });

  // Database parameters (if postgres enabled)
  if (config.postgres?.enabled) {
    parameters.push({
      name: `/${config.env}/${config.project}/backend/pg_database_password`,
      description: 'PostgreSQL password for backend access',
      type: 'SecureString',
      icon: Key,
      color: 'text-green-400',
    });
    
    parameters.push({
      name: `/${config.env}/${config.project}/postgres_password`,
      description: 'Main PostgreSQL database password',
      type: 'SecureString',
      icon: Key,
      color: 'text-green-400',
      note: 'Auto-generated random password'
    });

    // PgAdmin password (if enabled)
    if (config.workload?.install_pg_admin) {
      parameters.push({
        name: `/${config.env}/${config.project}/pgadmin_password`,
        description: 'PgAdmin access password',
        type: 'SecureString',
        icon: Key,
        color: 'text-green-400',
        note: 'Auto-generated random password'
      });
    }
  }

  // Firebase Cloud Messaging (if enabled)
  if (config.workload?.setup_fcnsns) {
    parameters.push({
      name: `/${config.env}/${config.project}/backend/gcm-server-key`,
      description: 'Google Cloud Messaging server key for push notifications',
      type: 'SecureString',
      icon: Key,
      color: 'text-yellow-400',
    });
  }

  // Additional services
  if (config.services && config.services.length > 0) {
    config.services.forEach((service) => {
      parameters.push({
        name: `/${config.env}/${config.project}/${service.name}/env`,
        description: `Service-specific environment configuration for ${service.name}`,
        type: 'SecureString',
        icon: Package,
        color: 'text-indigo-400',
        note: 'Initially blank'
      });
    });
  }

  // Scheduled tasks
  if (config.scheduled_tasks && config.scheduled_tasks.length > 0) {
    config.scheduled_tasks.forEach((task) => {
      parameters.push({
        name: `/${config.env}/${config.project}/task/${task.name}/env`,
        description: `Task-specific environment configuration for ${task.name}`,
        type: 'SecureString',
        icon: Clock,
        color: 'text-orange-400',
        note: 'Initially blank'
      });
    });
  }

  // Event processor tasks
  if (config.event_processor_tasks && config.event_processor_tasks.length > 0) {
    config.event_processor_tasks.forEach((task) => {
      parameters.push({
        name: `/${config.env}/${config.project}/task/${task.name}/env`,
        description: `Task-specific environment configuration for ${task.name}`,
        type: 'SecureString',
        icon: Clock,
        color: 'text-pink-400',
        note: 'Initially blank'
      });
    });
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>AWS Systems Manager Parameter Store</CardTitle>
          <CardDescription>
            Secure storage for configuration data and secrets
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* Key Design Patterns */}
          <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-blue-400 mt-0.5" />
              <div className="flex-1">
                <h4 className="text-sm font-medium text-blue-400 mb-2">Key Design Patterns</h4>
                <ul className="text-xs text-gray-300 space-y-1">
                  <li>• <strong>Hierarchical Organization:</strong> Parameters organized by /{config.env}/{config.project}/{'{component}'}/</li>
                  <li>• <strong>SecureString Encryption:</strong> All sensitive data encrypted using KMS</li>
                  <li>• <strong>Lifecycle Management:</strong> Parameters have <code className="text-blue-300">ignore_changes = [value]</code> to prevent Terraform overwriting manual updates</li>
                  <li>• <strong>Auto-Discovery:</strong> Services use <code className="text-blue-300">aws_ssm_parameters_by_path</code> to automatically load all parameters in their namespace</li>
                  <li>• <strong>Tagging:</strong> All parameters tagged with Environment, Project, ManagedBy, and Application</li>
                </ul>
              </div>
            </div>
          </div>

          {/* Parameters list */}
          <div>
            <h3 className="text-sm font-medium text-gray-300 mb-3">Parameters Created</h3>
            <div className="space-y-2">
              {parameters.map((param, index) => {
                const Icon = param.icon;
                return (
                  <div key={index} className="bg-gray-800 rounded-lg p-3">
                    <div className="flex items-start gap-3">
                      <Icon className={`w-4 h-4 ${param.color} mt-0.5`} />
                      <div className="flex-1 space-y-1">
                        <code className="text-xs text-gray-300 font-mono">{param.name}</code>
                        <p className="text-xs text-gray-500">{param.description}</p>
                        <div className="flex items-center gap-2">
                          <span className={`text-xs ${param.type === 'SecureString' ? 'text-green-400' : 'text-blue-400'}`}>
                            Type: {param.type}
                          </span>
                          {param.note && (
                            <>
                              <span className="text-xs text-gray-600">•</span>
                              <span className="text-xs text-gray-400">{param.note}</span>
                            </>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Parameter count */}
          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Total Parameters:</span>
              <span className="text-sm font-medium text-gray-300">{parameters.length}</span>
            </div>
          </div>

          {/* Auto-Discovery Example */}
          <div className="bg-gray-800 rounded-lg p-4">
            <h4 className="text-sm font-medium text-gray-300 mb-3">Auto-Discovery in ECS Tasks</h4>
            <pre className="text-xs text-gray-400 overflow-x-auto">
{`# ECS tasks automatically load all parameters from their path
# Example for backend service:
aws ssm get-parameters-by-path \\
  --path "/${config.env}/${config.project}/backend/" \\
  --recursive \\
  --with-decryption

# This loads all parameters:
# - /${config.env}/${config.project}/backend/env
# - /${config.env}/${config.project}/backend/pg_database_password
# - /${config.env}/${config.project}/backend/gcm-server-key (if enabled)`}</pre>
          </div>

          {/* Why this approach */}
          <div className="bg-green-900/20 border border-green-700 rounded-lg p-4">
            <h4 className="text-sm font-medium text-green-400 mb-2">Why This Approach?</h4>
            <ul className="text-xs text-gray-300 space-y-1">
              <li>• <strong>Security:</strong> Sensitive data like passwords and API keys are encrypted</li>
              <li>• <strong>Flexibility:</strong> Operators can manually add/update parameters without modifying Terraform</li>
              <li>• <strong>Separation:</strong> Each service has its own parameter namespace</li>
              <li>• <strong>Integration:</strong> ECS tasks automatically inject these as environment variables</li>
            </ul>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
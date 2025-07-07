import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Alert, AlertDescription } from './ui/alert';
import { Shield, Key, Lock, AlertCircle, Info, Calendar, Container, Cloud } from 'lucide-react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { ComponentNode } from '../types';

interface ScheduledTaskIAMPermissionsProps {
  config: YamlInfrastructureConfig;
  node: ComponentNode;
}

export function ScheduledTaskIAMPermissions({ config, node }: ScheduledTaskIAMPermissionsProps) {
  // Extract task name from node id
  const taskName = node.id.replace('scheduled-', '');
  
  // Check if SQS is enabled for this specific task
  const taskConfig = config.scheduled_tasks?.find(task => task.name === taskName);
  const sqsEnabled = taskConfig?.sqs_enable || false;

  const roles = [
    {
      name: `${config.project}_${taskName}_task_${config.env}`,
      type: 'Task Role',
      icon: Container,
      purpose: 'Used by the running container',
      permissions: [
        {
          name: 'CloudWatchFullAccess',
          description: 'Write logs and metrics',
          managed: true
        },
        {
          name: 'SSM Parameter Access',
          description: `Read parameters from /${config.env}/${config.project}/task/${taskName}/*`,
          managed: false,
          resource: `arn:aws:ssm:${config.region}:*:parameter/${config.env}/${config.project}/task/${taskName}/*`
        },
        ...(sqsEnabled ? [{
          name: 'SQS Access',
          description: 'Access to SQS queue (when sqs_enable is true)',
          managed: false,
          policyArn: config.sqs?.policy_arn || 'Configured via sqs_policy_arn'
        }] : [])
      ]
    },
    {
      name: `${config.project}_scheduler_${taskName}_task_execution_${config.env}`,
      type: 'Task Execution Role',
      icon: Cloud,
      purpose: 'Used by ECS to pull images and start containers',
      permissions: [
        {
          name: 'AmazonECSTaskExecutionRolePolicy',
          description: 'Pull ECR images, write logs',
          managed: true
        },
        {
          name: 'CloudWatchFullAccess',
          description: 'Create log streams',
          managed: true
        },
        {
          name: 'SSM Parameter Access',
          description: 'Read secrets for environment variables',
          managed: false,
          resource: `arn:aws:ssm:${config.region}:*:parameter/${config.env}/${config.project}/task/${taskName}/*`
        }
      ]
    },
    {
      name: `${config.project}_scheduler_${taskName}_role_${config.env}`,
      type: 'Scheduler Role',
      icon: Calendar,
      purpose: 'Used by EventBridge Scheduler to trigger the task',
      permissions: [
        {
          name: 'AmazonEventBridgeFullAccess',
          description: 'Manage schedules',
          managed: true
        },
        {
          name: 'AmazonECS_FullAccess',
          description: 'Run ECS tasks',
          managed: true
        }
      ]
    }
  ];

  return (
    <div className="space-y-4">
      {/* Overview */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Shield className="w-5 h-5" />
            IAM Roles Overview
          </CardTitle>
          <CardDescription>
            Three separate roles with specific permissions for scheduled task execution
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Alert>
            <Info className="h-4 w-4" />
            <AlertDescription>
              Each scheduled task uses three IAM roles following the principle of least privilege. 
              Roles are automatically created by Terraform with task-specific permissions.
            </AlertDescription>
          </Alert>
        </CardContent>
      </Card>

      {/* Roles */}
      {roles.map((role, index) => (
        <Card key={index}>
          <CardHeader>
            <div className="flex items-start justify-between">
              <div className="space-y-1">
                <CardTitle className="flex items-center gap-2 text-base">
                  <role.icon className="w-4 h-4" />
                  {role.type}
                </CardTitle>
                <code className="text-xs text-gray-400 font-mono">{role.name}</code>
              </div>
              <Badge variant="outline">IAM Role</Badge>
            </div>
            <CardDescription>{role.purpose}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              <h4 className="text-sm font-medium text-gray-300">Permissions</h4>
              <div className="space-y-2">
                {role.permissions.map((permission, permIndex) => (
                  <div key={permIndex} className="border border-gray-700 rounded-lg p-3">
                    <div className="flex items-start justify-between mb-1">
                      <div className="flex items-center gap-2">
                        <Key className="w-3 h-3 text-blue-400" />
                        <span className="text-sm font-medium text-gray-200">{permission.name}</span>
                      </div>
                      {permission.managed && (
                        <Badge variant="secondary" className="text-xs">AWS Managed</Badge>
                      )}
                    </div>
                    <p className="text-xs text-gray-400 mb-2">{permission.description}</p>
                    {permission.resource && (
                      <div className="mt-2 p-2 bg-gray-800 rounded">
                        <p className="text-xs text-gray-500 mb-1">Resource ARN:</p>
                        <code className="text-xs text-gray-300 break-all">{permission.resource}</code>
                      </div>
                    )}
                    {permission.policyArn && (
                      <div className="mt-2 p-2 bg-gray-800 rounded">
                        <p className="text-xs text-gray-500 mb-1">Policy ARN:</p>
                        <code className="text-xs text-gray-300">{permission.policyArn}</code>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      ))}

      {/* Security Notes */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Lock className="w-5 h-5" />
            Security Configuration
          </CardTitle>
          <CardDescription>
            Important security considerations for scheduled tasks
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-3">
            <div className="p-3 bg-gray-800 rounded-lg">
              <h4 className="text-sm font-medium text-gray-300 mb-2">Least Privilege Access</h4>
              <ul className="text-xs text-gray-400 space-y-1 ml-4">
                <li>• Roles follow least-privilege principle</li>
                <li>• SSM access is scoped to task-specific paths only</li>
                <li>• No cross-task parameter access</li>
                <li>• Tasks cannot access other services' parameters or resources</li>
              </ul>
            </div>

            <div className="p-3 bg-gray-800 rounded-lg">
              <h4 className="text-sm font-medium text-gray-300 mb-2">Parameter Store Access Pattern</h4>
              <div className="bg-gray-900 rounded p-2 mt-2">
                <code className="text-xs text-gray-300">
                  /{config.env}/{config.project}/task/{taskName}/*
                </code>
              </div>
              <p className="text-xs text-gray-400 mt-2">
                Tasks can only read/write parameters under their designated namespace
              </p>
            </div>

            {sqsEnabled && (
              <div className="p-3 bg-blue-900/20 border border-blue-700 rounded-lg">
                <h4 className="text-sm font-medium text-blue-400 mb-2 flex items-center gap-2">
                  <Info className="w-3 h-3" />
                  SQS Access Enabled
                </h4>
                <p className="text-xs text-gray-300">
                  This task has SQS access enabled. The task role includes the SQS policy 
                  specified in your configuration.
                </p>
              </div>
            )}

            <div className="p-3 bg-yellow-900/20 border border-yellow-700 rounded-lg">
              <h4 className="text-sm font-medium text-yellow-400 mb-2 flex items-center gap-2">
                <AlertCircle className="w-3 h-3" />
                CloudWatch Permissions Note
              </h4>
              <p className="text-xs text-gray-300">
                CloudWatch permissions are quite broad to ensure proper logging and monitoring. 
                Consider using more restrictive policies in production if needed.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Trust Relationships */}
      <Card>
        <CardHeader>
          <CardTitle>Trust Relationships</CardTitle>
          <CardDescription>
            Services that can assume these roles
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div className="p-3 bg-gray-800 rounded-lg">
              <p className="text-sm font-medium text-gray-300 mb-2">Task & Execution Roles</p>
              <code className="text-xs text-gray-400">ecs-tasks.amazonaws.com</code>
            </div>
            <div className="p-3 bg-gray-800 rounded-lg">
              <p className="text-sm font-medium text-gray-300 mb-2">Scheduler Role</p>
              <code className="text-xs text-gray-400">scheduler.amazonaws.com</code>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
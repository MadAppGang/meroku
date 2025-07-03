import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Shield, Check, X, AlertCircle } from 'lucide-react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';

interface BackendIAMPermissionsProps {
  config: YamlInfrastructureConfig;
}

export function BackendIAMPermissions({ config }: BackendIAMPermissionsProps) {
  const permissions = [
    {
      service: 'S3',
      resource: `${config.project}-backend-${config.env}-*`,
      actions: ['s3:GetObject', 's3:PutObject', 's3:DeleteObject', 's3:ListBucket'],
      enabled: true,
      description: 'Read/write access to backend S3 bucket',
    },
    {
      service: 'Parameter Store',
      resource: `/${config.env}/${config.project}/backend/*`,
      actions: ['ssm:GetParameter', 'ssm:GetParameters', 'ssm:GetParametersByPath'],
      enabled: true,
      description: 'Read access to backend parameters',
    },
    {
      service: 'CloudWatch Logs',
      resource: `/aws/ecs/${config.project}_service_${config.env}`,
      actions: ['logs:CreateLogStream', 'logs:PutLogEvents'],
      enabled: true,
      description: 'Write logs to CloudWatch',
    },
    {
      service: 'SQS',
      resource: 'SQS Queue',
      actions: ['sqs:SendMessage', 'sqs:ReceiveMessage', 'sqs:DeleteMessage'],
      enabled: !!config.sqs,
      description: 'Send and receive messages from SQS',
    },
    {
      service: 'X-Ray',
      resource: '*',
      actions: ['xray:PutTraceSegments', 'xray:PutTelemetryRecords'],
      enabled: config.workload?.xray_enabled === true,
      description: 'Send traces to AWS X-Ray',
    },
    {
      service: 'SES',
      resource: config.ses?.domain_name || '*',
      actions: ['ses:SendEmail', 'ses:SendRawEmail'],
      enabled: config.ses?.enabled === true,
      description: 'Send emails via SES',
    },
    {
      service: 'SNS',
      resource: 'FCM Platform Application',
      actions: ['sns:Publish', 'sns:CreatePlatformEndpoint', 'sns:DeleteEndpoint'],
      enabled: config.workload?.setup_fcnsns === true,
      description: 'Send push notifications via FCM/SNS',
    },
    {
      service: 'ECS Execute Command',
      resource: 'ECS Tasks',
      actions: ['ecs:ExecuteCommand', 'ssmmessages:*'],
      enabled: config.workload?.backend_remote_access === true,
      description: 'Remote shell access to containers',
    },
  ];

  const enabledPermissions = permissions.filter(p => p.enabled);
  const disabledPermissions = permissions.filter(p => !p.enabled);

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Shield className="w-5 h-5" />
            Task IAM Role
          </CardTitle>
          <CardDescription>
            <code className="font-mono text-sm">{config.project}-backend-task-role-{config.env}</code>
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="p-3 bg-gray-800 rounded">
            <p className="text-sm text-gray-300 mb-2">The ECS task assumes this IAM role to access AWS services.</p>
            <p className="text-xs text-gray-400">ARN: <code className="font-mono">arn:aws:iam::ACCOUNT:role/{config.project}-backend-task-role-{config.env}</code></p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Active Permissions</CardTitle>
          <CardDescription>AWS services the backend can access</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {enabledPermissions.map((perm, index) => (
              <div key={index} className="border border-gray-700 rounded p-3 space-y-2">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2">
                    <Check className="w-4 h-4 text-green-400" />
                    <span className="font-medium text-sm">{perm.service}</span>
                  </div>
                  <Badge variant="default" className="text-xs">Active</Badge>
                </div>
                <p className="text-xs text-gray-400">{perm.description}</p>
                <div className="space-y-1">
                  <p className="text-xs text-gray-500">Resource: <code className="font-mono">{perm.resource}</code></p>
                  <div className="flex flex-wrap gap-1 mt-1">
                    {perm.actions.map((action, idx) => (
                      <Badge key={idx} variant="secondary" className="text-xs font-mono">
                        {action}
                      </Badge>
                    ))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {disabledPermissions.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Inactive Permissions</CardTitle>
            <CardDescription>Services not enabled in current configuration</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3 opacity-60">
              {disabledPermissions.map((perm, index) => (
                <div key={index} className="border border-gray-700 rounded p-3 space-y-2">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-2">
                      <X className="w-4 h-4 text-gray-400" />
                      <span className="font-medium text-sm">{perm.service}</span>
                    </div>
                    <Badge variant="outline" className="text-xs">Inactive</Badge>
                  </div>
                  <p className="text-xs text-gray-400">{perm.description}</p>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Security Best Practices</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-2 text-sm">
            <div className="flex items-start gap-2">
              <AlertCircle className="w-4 h-4 text-yellow-400 mt-0.5" />
              <div>
                <p className="text-gray-300">Least Privilege Access</p>
                <p className="text-xs text-gray-400">Task role only has permissions for required services</p>
              </div>
            </div>
            <div className="flex items-start gap-2">
              <AlertCircle className="w-4 h-4 text-yellow-400 mt-0.5" />
              <div>
                <p className="text-gray-300">Resource-Specific Permissions</p>
                <p className="text-xs text-gray-400">Permissions are scoped to specific resources where possible</p>
              </div>
            </div>
            <div className="flex items-start gap-2">
              <AlertCircle className="w-4 h-4 text-yellow-400 mt-0.5" />
              <div>
                <p className="text-gray-300">Separate Execution Role</p>
                <p className="text-xs text-gray-400">Task execution role is separate from task role</p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Bell, Key, Smartphone, Server } from 'lucide-react';
import type { YamlInfrastructureConfig } from '../types/yamlConfig';

interface BackendSNSProps {
  config: YamlInfrastructureConfig;
}

export function BackendSNS({ config }: BackendSNSProps) {
  const platformAppName = `${config.project}-fcm-${config.env}`;
  const gcmKeyPath = `/${config.env}/${config.project}/backend/gcm-server-key`;
  const isEnabled = config.workload?.setup_fcnsns === true;

  if (!isEnabled) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Bell className="w-5 h-5" />
            Amazon SNS
          </CardTitle>
          <CardDescription>Firebase Cloud Messaging / Push Notifications</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="text-center py-8 text-gray-400">
            <Bell className="w-8 h-8 mx-auto mb-2 opacity-50" />
            <p className="text-sm">SNS is not enabled</p>
            <p className="text-xs mt-1">Set setup_fcnsns: true in your environment YAML file to enable</p>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Bell className="w-5 h-5" />
            Amazon SNS - Firebase Cloud Messaging
          </CardTitle>
          <CardDescription>Push notification platform application</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="flex items-center justify-between p-3 bg-gray-800 rounded">
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <Smartphone className="w-4 h-4 text-pink-400" />
                  <code className="text-sm font-mono text-pink-400">{platformAppName}</code>
                </div>
                <div className="flex items-center gap-4 mt-2">
                  <Badge variant="outline" className="text-xs">GCM Platform</Badge>
                  <Badge variant="success" className="text-xs">Active</Badge>
                </div>
              </div>
            </div>

            <div className="space-y-2 text-sm text-gray-400">
              <p className="flex items-center gap-2">
                <span className="text-gray-500">Platform:</span>
                <span>Google Cloud Messaging (GCM/FCM)</span>
              </p>
              <p className="flex items-center gap-2">
                <span className="text-gray-500">Purpose:</span>
                <span>Mobile push notifications</span>
              </p>
            </div>

            <div className="p-3 bg-gray-800 rounded space-y-2">
              <p className="text-xs font-medium text-gray-300">GCM Server Key Location:</p>
              <div className="flex items-center gap-2">
                <Key className="w-3 h-3 text-gray-400" />
                <code className="text-xs font-mono text-blue-400">{gcmKeyPath}</code>
              </div>
              <p className="text-xs text-gray-500 mt-1">
                Stored in AWS Systems Manager Parameter Store (SecureString)
              </p>
            </div>

            <div className="p-3 bg-gray-800 rounded space-y-2">
              <p className="text-xs font-medium text-gray-300">IAM Permissions:</p>
              <ul className="text-xs text-gray-400 space-y-1 ml-4">
                <li>• sns:Publish</li>
                <li>• sns:CreatePlatformEndpoint</li>
                <li>• sns:DeleteEndpoint</li>
                <li>• sns:GetEndpointAttributes</li>
                <li>• sns:SetEndpointAttributes</li>
              </ul>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <Server className="w-4 h-4" />
            Integration Details
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="space-y-2 text-sm">
              <h4 className="font-medium text-gray-200">Backend Integration</h4>
              <div className="pl-4 space-y-1 text-gray-400">
                <p>• Platform application ARN available via environment</p>
                <p>• Create platform endpoints for device tokens</p>
                <p>• Send push notifications to mobile devices</p>
                <p>• Manage endpoint attributes and subscriptions</p>
              </div>
            </div>

            <div className="space-y-2 text-sm">
              <h4 className="font-medium text-gray-200">Required Setup</h4>
              <div className="pl-4 space-y-1 text-gray-400">
                <p>1. Add your FCM server key to Parameter Store:</p>
                <code className="block ml-4 text-xs bg-gray-900 p-2 rounded font-mono">
                  aws ssm put-parameter --name "{gcmKeyPath}" \<br />
                  --value "YOUR_FCM_SERVER_KEY" --type SecureString
                </code>
                <p className="mt-2">2. Enable in your environment YAML:</p>
                <code className="block ml-4 text-xs bg-gray-900 p-2 rounded font-mono">
                  setup_fcnsns: true
                </code>
              </div>
            </div>

            <div className="p-3 bg-yellow-900/20 border border-yellow-700 rounded">
              <p className="text-xs text-yellow-300">
                <strong>Note:</strong> Currently supports FCM (Firebase Cloud Messaging) only. 
                For other SNS features like topics, email, or SMS, additional configuration would be needed.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
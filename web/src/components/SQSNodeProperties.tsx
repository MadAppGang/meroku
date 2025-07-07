import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Switch } from './ui/switch';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { Alert, AlertDescription } from './ui/alert';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { 
  MessageSquare, 
  CheckCircle,
  Info,
  ExternalLink,
  Zap,
  Server,
  Database,
  Clock
} from 'lucide-react';

interface SQSNodePropertiesProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
  accountInfo?: { accountId: string; region: string; profile: string };
}

export function SQSNodeProperties({ config, onConfigChange, accountInfo }: SQSNodePropertiesProps) {
  const sqsConfig = config.sqs || { enabled: false };
  
  const handleToggleSQS = (enabled: boolean) => {
    onConfigChange({
      sqs: {
        ...sqsConfig,
        enabled
      }
    });
  };

  const handleUpdateConfig = (updates: Partial<typeof sqsConfig>) => {
    onConfigChange({
      sqs: {
        ...sqsConfig,
        ...updates
      }
    });
  };

  // Determine actual values with defaults
  const actualQueueName = sqsConfig.name || 'default-queue';
  const queueUrl = `https://sqs.${config.region}.amazonaws.com/${accountInfo?.accountId || '<ACCOUNT_ID>'}/${config.project}-${config.env}-${actualQueueName}`;

  return (
    <div className="space-y-6">
      {/* Enable/Disable SQS */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <MessageSquare className="w-5 h-5" />
            Amazon SQS
          </CardTitle>
          <CardDescription>
            Simple Queue Service for asynchronous task processing
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <Label htmlFor="sqs-enabled" className="text-base">Enable SQS</Label>
              <p className="text-sm text-gray-500">
                Create a managed message queue for async processing
              </p>
            </div>
            <Switch
              id="sqs-enabled"
              checked={sqsConfig.enabled}
              onCheckedChange={handleToggleSQS}
            />
          </div>
        </CardContent>
      </Card>

      {sqsConfig.enabled && (
        <>
          {/* Queue Configuration */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Server className="w-5 h-5" />
                Queue Configuration
              </CardTitle>
              <CardDescription>
                Configure your SQS message queue settings
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="queue-name">Queue Name</Label>
                <Input
                  id="queue-name"
                  value={sqsConfig.name || ''}
                  onChange={(e) => handleUpdateConfig({ name: e.target.value })}
                  placeholder="default-queue"
                />
                <p className="text-xs text-gray-500">
                  Default: default-queue
                </p>
              </div>

              <div className="bg-gray-800 rounded-lg p-3">
                <h4 className="text-sm font-medium text-gray-200 mb-2">Queue Details</h4>
                <div className="space-y-2 text-xs">
                  <div>
                    <span className="text-gray-400">Full Queue Name:</span>
                    <div className="font-mono text-blue-400 break-all">
                      {config.project}-{config.env}-{actualQueueName}
                    </div>
                  </div>
                  <div>
                    <span className="text-gray-400">Queue URL:</span>
                    <div className="font-mono text-green-400 break-all">
                      {queueUrl}
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Integration Details */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Zap className="w-5 h-5" />
                Auto-Integration
              </CardTitle>
              <CardDescription>
                SQS automatically integrates with these components when enabled
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="grid grid-cols-1 gap-3">
                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">Backend Service</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        SQS_QUEUE_URL environment variable automatically added
                      </p>
                    </div>
                  </div>

                  {config.scheduled_tasks && config.scheduled_tasks.length > 0 && (
                    <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                      <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                      <div className="flex-1">
                        <h4 className="text-sm font-medium text-gray-200">Scheduled Tasks</h4>
                        <p className="text-xs text-gray-400 mt-1">
                          Queue access and policies automatically configured
                        </p>
                      </div>
                    </div>
                  )}

                  {config.event_processor_tasks && config.event_processor_tasks.length > 0 && (
                    <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                      <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                      <div className="flex-1">
                        <h4 className="text-sm font-medium text-gray-200">Event Processor Tasks</h4>
                        <p className="text-xs text-gray-400 mt-1">
                          Queue access and policies automatically configured
                        </p>
                      </div>
                    </div>
                  )}

                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">IAM Policies</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        SQS access policies automatically created and attached
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Environment Variables */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Database className="w-5 h-5" />
                Environment Variables
              </CardTitle>
              <CardDescription>
                Variables automatically injected into services
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                <div className="bg-gray-800 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-gray-200 mb-2">Backend Service</h4>
                  <div className="space-y-2">
                    <div>
                      <div className="flex items-center justify-between mb-1">
                        <span className="text-sm font-mono text-blue-400">SQS_QUEUE_URL</span>
                        <Badge variant="outline" className="text-xs">Auto-injected</Badge>
                      </div>
                      <div className="text-xs font-mono text-gray-300 break-all bg-gray-900 p-2 rounded">
                        {queueUrl}
                      </div>
                    </div>
                  </div>
                </div>

                {(config.scheduled_tasks?.length || config.event_processor_tasks?.length) && (
                  <div className="bg-gray-800 rounded-lg p-3">
                    <h4 className="text-sm font-medium text-gray-200 mb-2">Tasks</h4>
                    <p className="text-xs text-gray-400">
                      SQS queue URL and access policies are automatically provided to all scheduled tasks and event processor tasks.
                    </p>
                  </div>
                )}
              </div>
            </CardContent>
          </Card>

          {/* Queue Features */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Clock className="w-5 h-5" />
                Queue Features
              </CardTitle>
              <CardDescription>
                Built-in SQS capabilities and configurations
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="grid grid-cols-1 gap-3">
                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">Standard Queue</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        At-least-once delivery with high throughput
                      </p>
                    </div>
                  </div>

                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">Dead Letter Queue</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        Automatic DLQ for failed message processing
                      </p>
                    </div>
                  </div>

                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">Encryption</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        Server-side encryption with AWS managed keys
                      </p>
                    </div>
                  </div>

                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">CloudWatch Integration</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        Metrics for queue depth, message age, and throughput
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Important Notes */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Info className="w-5 h-5" />
                Important Notes
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="space-y-2 text-sm text-gray-300">
                <p>• Queue URL is automatically provided to backend services and tasks</p>
                <p>• IAM policies for SQS access are automatically created and attached</p>
                <p>• Dead letter queue is automatically configured for failed messages</p>
                <p>• Queue is created in the same region as your infrastructure</p>
                <p>• Message retention period is set to 14 days by default</p>
              </div>

              <div className="pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => window.open(`https://console.aws.amazon.com/sqs/v2/home?region=${config.region}#/queues`, '_blank')}
                >
                  <ExternalLink className="w-4 h-4 mr-2" />
                  Open SQS Console
                </Button>
              </div>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
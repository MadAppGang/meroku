import React, { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Alert, AlertDescription } from './ui/alert';
import { Cloud, Terminal, Copy, Info, Clock, FileText } from 'lucide-react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { ComponentNode } from '../types';

interface BackendCloudWatchProps {
  config: YamlInfrastructureConfig;
  node?: ComponentNode;
}

export function BackendCloudWatch({ config, node }: BackendCloudWatchProps) {
  // Determine if this is for a service or backend
  const isService = node?.type === 'service';
  const serviceName = isService ? node.id.replace('service-', '') : null;
  const [copiedCommand, setCopiedCommand] = useState<string | null>(null);

  const logGroups = isService ? [
    {
      name: `${config.project}_service_${serviceName}_${config.env}`,
      description: `${serviceName} service logs`,
      service: `ECS ${serviceName} Service`,
      retention: '7 days',
    }
  ] : [
    {
      name: `${config.project}_backend_${config.env}`,
      description: 'Backend service logs',
      service: 'ECS Backend Service',
      retention: '7 days',
    },
    {
      name: `${config.project}_adot_collector_${config.env}`,
      description: 'OpenTelemetry collector logs',
      service: 'ADOT Sidecar Container',
      retention: '7 days',
    }
  ];

  const copyCommand = (command: string, id: string) => {
    navigator.clipboard.writeText(command);
    setCopiedCommand(id);
    setTimeout(() => setCopiedCommand(null), 2000);
  };

  const generateTailCommand = (logGroup: string) => {
    return `aws logs tail ${logGroup} --follow`;
  };

  const generateFilterCommand = (logGroup: string) => {
    return `aws logs filter-log-events \\
  --log-group-name "${logGroup}" \\
  --start-time $(date -d '30 minutes ago' +%s)000`;
  };

  const generateFilterCommandWithQuery = (logGroup: string) => {
    return `aws logs filter-log-events \\
  --log-group-name "${logGroup}" \\
  --filter-pattern "[ERROR]" \\
  --start-time $(date -d '1 hour ago' +%s)000`;
  };

  const generateInsightsQuery = (logGroup: string) => {
    return `fields @timestamp, @message
| filter @message like /ERROR/
| sort @timestamp desc
| limit 100`;
  };

  return (
    <div className="space-y-4">
      {/* Log Groups Overview */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Cloud className="w-5 h-5" />
            CloudWatch Log Groups
          </CardTitle>
          <CardDescription>
            Log groups for {isService ? `${serviceName} service` : 'backend services and collectors'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {logGroups.map((group, index) => (
              <div key={index} className="border border-gray-700 rounded-lg p-4 space-y-3">
                <div className="flex items-start justify-between">
                  <div>
                    <h4 className="text-sm font-medium text-gray-200 flex items-center gap-2">
                      <FileText className="w-4 h-4 text-blue-400" />
                      {group.name}
                    </h4>
                    <p className="text-xs text-gray-400 mt-1">{group.description}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className="text-xs">
                      {group.service}
                    </Badge>
                    <Badge variant="secondary" className="text-xs">
                      <Clock className="w-3 h-3 mr-1" />
                      {group.retention}
                    </Badge>
                  </div>
                </div>
                
                <div className="grid grid-cols-1 gap-2 text-xs">
                  <div className="flex items-center gap-2 text-gray-400">
                    <span className="font-medium">Service:</span>
                    <code className="text-blue-400">{group.name}</code>
                  </div>
                  <div className="flex items-center gap-2 text-gray-400">
                    <span className="font-medium">Full Path:</span>
                    <code className="text-green-400">{group.name}</code>
                  </div>
                </div>
              </div>
            ))}
          </div>

          <Alert className="mt-4">
            <Info className="h-4 w-4" />
            <AlertDescription>
              Logs are automatically created when services start. Retention is set to 7 days by default.
              Log streams are created per task instance.
            </AlertDescription>
          </Alert>
        </CardContent>
      </Card>

      {/* CLI Commands */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Terminal className="w-5 h-5" />
            CloudWatch CLI Commands
          </CardTitle>
          <CardDescription>
            Useful AWS CLI commands for viewing and filtering logs
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {logGroups.map((group, groupIndex) => (
            <div key={groupIndex} className="space-y-4">
              <h4 className="text-sm font-medium text-gray-300 border-b border-gray-700 pb-2">
                {group.name} Commands
              </h4>
              
              {/* Tail Logs */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-xs font-medium text-gray-400">Tail logs (real-time)</Label>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => copyCommand(generateTailCommand(group.name), `tail-${groupIndex}`)}
                    className="h-6 px-2 text-xs"
                  >
                    <Copy className="w-3 h-3 mr-1" />
                    {copiedCommand === `tail-${groupIndex}` ? 'Copied!' : 'Copy'}
                  </Button>
                </div>
                <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                  <pre>{generateTailCommand(group.name)}</pre>
                </div>
              </div>

              {/* Filter Last 30 Minutes */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-xs font-medium text-gray-400">Filter logs (last 30 minutes)</Label>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => copyCommand(generateFilterCommand(group.name), `filter-${groupIndex}`)}
                    className="h-6 px-2 text-xs"
                  >
                    <Copy className="w-3 h-3 mr-1" />
                    {copiedCommand === `filter-${groupIndex}` ? 'Copied!' : 'Copy'}
                  </Button>
                </div>
                <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300 overflow-x-auto">
                  <pre>{generateFilterCommand(group.name)}</pre>
                </div>
              </div>

              {/* Filter Errors */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-xs font-medium text-gray-400">Filter ERROR logs (last hour)</Label>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => copyCommand(generateFilterCommandWithQuery(group.name), `error-${groupIndex}`)}
                    className="h-6 px-2 text-xs"
                  >
                    <Copy className="w-3 h-3 mr-1" />
                    {copiedCommand === `error-${groupIndex}` ? 'Copied!' : 'Copy'}
                  </Button>
                </div>
                <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300 overflow-x-auto">
                  <pre>{generateFilterCommandWithQuery(group.name)}</pre>
                </div>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>

      {/* CloudWatch Insights */}
      <Card>
        <CardHeader>
          <CardTitle>CloudWatch Insights</CardTitle>
          <CardDescription>
            Sample queries for analyzing logs in CloudWatch Insights
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <p className="text-xs text-gray-300 mb-3">
              CloudWatch Insights provides a powerful query language for analyzing log data. 
              Here are some useful queries:
            </p>
          </div>

          {/* Sample Queries */}
          <div className="space-y-4">
            <div className="space-y-2">
              <Label className="text-xs font-medium text-gray-400">Find all errors</Label>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{generateInsightsQuery('your-log-group')}</pre>
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-xs font-medium text-gray-400">Response time analysis</Label>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{`fields @timestamp, @message
| filter @message like /response_time/
| parse @message /response_time=(?<duration>\\d+)/
| stats avg(duration), max(duration), min(duration) by bin(5m)`}</pre>
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-xs font-medium text-gray-400">Top error messages</Label>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{`fields @timestamp, @message
| filter @message like /ERROR/
| stats count() by @message
| sort count() desc
| limit 10`}</pre>
              </div>
            </div>
          </div>

          <Alert>
            <Terminal className="h-4 w-4" />
            <AlertDescription>
              You can run these queries in the AWS Console under CloudWatch → Logs → Insights.
              Select your log group and paste the query.
            </AlertDescription>
          </Alert>
        </CardContent>
      </Card>

      {/* Additional CLI Tips */}
      <Card>
        <CardHeader>
          <CardTitle>CLI Tips & Tricks</CardTitle>
          <CardDescription>
            Additional useful commands and options
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-3">
            <div className="p-3 bg-gray-800 rounded-lg space-y-2">
              <p className="text-sm font-medium text-gray-300">Common Options</p>
              <ul className="text-xs text-gray-400 space-y-1 ml-4">
                <li>• <code className="text-blue-400">--format short</code> - Simplified output format</li>
                <li>• <code className="text-blue-400">--since 1h</code> - Show logs from last hour</li>
                <li>• <code className="text-blue-400">--filter-pattern "[ERROR]"</code> - Filter specific patterns</li>
                <li>• <code className="text-blue-400">--profile your-profile</code> - Use specific AWS profile</li>
                <li>• <code className="text-blue-400">--region us-east-1</code> - Specify AWS region</li>
              </ul>
            </div>

            <div className="p-3 bg-gray-800 rounded-lg space-y-2">
              <p className="text-sm font-medium text-gray-300">Export Logs</p>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{`# Export logs to S3
aws logs create-export-task \\
  --log-group-name "${isService ? `${config.project}_service_${serviceName}_${config.env}` : `${config.project}_backend_${config.env}`}" \\
  --from $(date -d '7 days ago' +%s)000 \\
  --to $(date +%s)000 \\
  --destination "${config.project}-logs-export" \\
  --destination-prefix "${isService ? `service/${serviceName}` : 'backend'}/${config.env}/"`}</pre>
              </div>
            </div>

            <div className="p-3 bg-gray-800 rounded-lg space-y-2">
              <p className="text-sm font-medium text-gray-300">Stream to Local File</p>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{`# Save logs to local file
aws logs tail ${isService ? `${config.project}_service_${serviceName}_${config.env}` : `${config.project}_backend_${config.env}`} \\
  --follow \\
  --format short > ${isService ? `${serviceName}` : 'backend'}-logs-$(date +%Y%m%d-%H%M%S).log`}</pre>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function Label({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <div className={`text-sm font-medium ${className}`}>{children}</div>;
}
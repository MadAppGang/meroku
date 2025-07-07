import React, { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Alert, AlertDescription } from './ui/alert';
import { Cloud, Terminal, Copy, Info, Clock, FileText } from 'lucide-react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { ComponentNode } from '../types';

interface ScheduledTaskCloudWatchProps {
  config: YamlInfrastructureConfig;
  node: ComponentNode;
}

export function ScheduledTaskCloudWatch({ config, node }: ScheduledTaskCloudWatchProps) {
  const [copiedCommand, setCopiedCommand] = useState<string | null>(null);
  
  // Extract task name from node id (works for both scheduled and event tasks)
  const taskName = node.id.replace(/^(scheduled|event)-/, '');

  const logGroup = {
    name: `${config.project}_task_${taskName}_${config.env}`,
    description: `Task logs for ${taskName}`,
    service: node.type === 'scheduled-task' ? 'ECS Scheduled Task' : 'ECS Event Task',
    retention: '7 days'
  };

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
  --start-time $(date -d '24 hours ago' +%s)000`;
  };

  const generateLastExecutionCommand = (logGroup: string) => {
    return `aws logs describe-log-streams \\
  --log-group-name "${logGroup}" \\
  --order-by LastEventTime \\
  --descending \\
  --limit 1`;
  };

  const generateFilterCommandWithQuery = (logGroup: string) => {
    return `aws logs filter-log-events \\
  --log-group-name "${logGroup}" \\
  --filter-pattern "[ERROR]" \\
  --start-time $(date -d '7 days ago' +%s)000`;
  };

  const generateInsightsQuery = () => {
    return `fields @timestamp, @message
| filter @message like /ERROR/
| sort @timestamp desc
| limit 100`;
  };

  const generateExecutionStatsQuery = () => {
    return `fields @timestamp, @message
| filter @message like /Task started/ or @message like /Task completed/
| stats count() by bin(1d)`;
  };

  return (
    <div className="space-y-4">
      {/* Log Group Overview */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Cloud className="w-5 h-5" />
            CloudWatch Log Group
          </CardTitle>
          <CardDescription>
            Log configuration for scheduled task: {taskName}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="border border-gray-700 rounded-lg p-4 space-y-3">
            <div className="flex items-start justify-between">
              <div>
                <h4 className="text-sm font-medium text-gray-200 flex items-center gap-2">
                  <FileText className="w-4 h-4 text-blue-400" />
                  {logGroup.name}
                </h4>
                <p className="text-xs text-gray-400 mt-1">{logGroup.description}</p>
              </div>
              <div className="flex flex-col items-end gap-2">
                <Badge variant="outline" className="text-xs">
                  {logGroup.service}
                </Badge>
                <Badge variant="secondary" className="text-xs">
                  <Clock className="w-3 h-3 mr-1" />
                  {logGroup.retention}
                </Badge>
              </div>
            </div>
            
            <div className="grid grid-cols-1 gap-2 text-xs">
              <div className="flex items-center gap-2 text-gray-400">
                <span className="font-medium">Task Name:</span>
                <code className="text-blue-400">{taskName}</code>
              </div>
              <div className="flex items-center gap-2 text-gray-400">
                <span className="font-medium">Log Path:</span>
                <code className="text-green-400">{logGroup.name}</code>
              </div>
            </div>
          </div>

          <Alert className="mt-4">
            <Info className="h-4 w-4" />
            <AlertDescription>
              Each task execution creates a new log stream. Logs are retained for 7 days. 
              Task runs are triggered based on the schedule expression.
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
            Commands for viewing and analyzing task logs
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* Get Last Execution */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-medium text-gray-400">Get last execution info</Label>
              <Button
                size="sm"
                variant="outline"
                onClick={() => copyCommand(generateLastExecutionCommand(logGroup.name), 'last-exec')}
                className="h-6 px-2 text-xs"
              >
                <Copy className="w-3 h-3 mr-1" />
                {copiedCommand === 'last-exec' ? 'Copied!' : 'Copy'}
              </Button>
            </div>
            <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300 overflow-x-auto">
              <pre>{generateLastExecutionCommand(logGroup.name)}</pre>
            </div>
          </div>

          {/* Tail Logs */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-medium text-gray-400">Tail logs (real-time)</Label>
              <Button
                size="sm"
                variant="outline"
                onClick={() => copyCommand(generateTailCommand(logGroup.name), 'tail')}
                className="h-6 px-2 text-xs"
              >
                <Copy className="w-3 h-3 mr-1" />
                {copiedCommand === 'tail' ? 'Copied!' : 'Copy'}
              </Button>
            </div>
            <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
              <pre>{generateTailCommand(logGroup.name)}</pre>
            </div>
          </div>

          {/* Filter Last 24 Hours */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-medium text-gray-400">View logs from last 24 hours</Label>
              <Button
                size="sm"
                variant="outline"
                onClick={() => copyCommand(generateFilterCommand(logGroup.name), 'filter')}
                className="h-6 px-2 text-xs"
              >
                <Copy className="w-3 h-3 mr-1" />
                {copiedCommand === 'filter' ? 'Copied!' : 'Copy'}
              </Button>
            </div>
            <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300 overflow-x-auto">
              <pre>{generateFilterCommand(logGroup.name)}</pre>
            </div>
          </div>

          {/* Filter Errors */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-medium text-gray-400">Find ERROR logs (last 7 days)</Label>
              <Button
                size="sm"
                variant="outline"
                onClick={() => copyCommand(generateFilterCommandWithQuery(logGroup.name), 'error')}
                className="h-6 px-2 text-xs"
              >
                <Copy className="w-3 h-3 mr-1" />
                {copiedCommand === 'error' ? 'Copied!' : 'Copy'}
              </Button>
            </div>
            <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300 overflow-x-auto">
              <pre>{generateFilterCommandWithQuery(logGroup.name)}</pre>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* CloudWatch Insights */}
      <Card>
        <CardHeader>
          <CardTitle>CloudWatch Insights</CardTitle>
          <CardDescription>
            Queries for analyzing scheduled task execution patterns
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <p className="text-xs text-gray-300 mb-3">
              Use CloudWatch Insights to analyze task execution patterns, errors, and performance.
            </p>
          </div>

          {/* Sample Queries */}
          <div className="space-y-4">
            <div className="space-y-2">
              <Label className="text-xs font-medium text-gray-400">Find all task errors</Label>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{generateInsightsQuery()}</pre>
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-xs font-medium text-gray-400">Task execution statistics</Label>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{generateExecutionStatsQuery()}</pre>
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-xs font-medium text-gray-400">Task duration analysis</Label>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{`fields @timestamp, @message
| filter @message like /Task completed in/
| parse @message /Task completed in (?<duration>\\d+)ms/
| stats avg(duration), max(duration), min(duration) by bin(1d)`}</pre>
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-xs font-medium text-gray-400">Failed executions</Label>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{`fields @timestamp, @message
| filter @message like /Task failed/ or @message like /ERROR/
| sort @timestamp desc
| limit 50`}</pre>
              </div>
            </div>
          </div>

          <Alert>
            <Terminal className="h-4 w-4" />
            <AlertDescription>
              Run these queries in AWS Console → CloudWatch → Logs → Insights.
              Select log group <code className="text-xs">{logGroup.name}</code> and paste the query.
            </AlertDescription>
          </Alert>
        </CardContent>
      </Card>

      {/* Task-Specific Tips */}
      <Card>
        <CardHeader>
          <CardTitle>Scheduled Task Monitoring Tips</CardTitle>
          <CardDescription>
            Best practices for monitoring scheduled tasks
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-3">
            <div className="p-3 bg-gray-800 rounded-lg space-y-2">
              <p className="text-sm font-medium text-gray-300">Monitoring Task Executions</p>
              <ul className="text-xs text-gray-400 space-y-1 ml-4">
                <li>• Check CloudWatch Events for task trigger history</li>
                <li>• Monitor task execution duration trends</li>
                <li>• Set up alarms for failed executions</li>
                <li>• Track resource usage (CPU/Memory) per execution</li>
              </ul>
            </div>

            <div className="p-3 bg-gray-800 rounded-lg space-y-2">
              <p className="text-sm font-medium text-gray-300">View Recent Executions</p>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{`# List all log streams (executions) from last 7 days
aws logs describe-log-streams \\
  --log-group-name "${logGroup.name}" \\
  --order-by LastEventTime \\
  --descending \\
  --limit 10 \\
  --query 'logStreams[*].[logStreamName,lastEventTime]' \\
  --output table`}</pre>
              </div>
            </div>

            <div className="p-3 bg-gray-800 rounded-lg space-y-2">
              <p className="text-sm font-medium text-gray-300">Export Task Logs</p>
              <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300">
                <pre>{`# Export task logs to S3
aws logs create-export-task \\
  --log-group-name "${logGroup.name}" \\
  --from $(date -d '30 days ago' +%s)000 \\
  --to $(date +%s)000 \\
  --destination "${config.project}-logs-export" \\
  --destination-prefix "scheduled-tasks/${taskName}/"`}</pre>
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
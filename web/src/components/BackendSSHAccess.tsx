import React, { useState, useEffect } from "react";
import { YamlInfrastructureConfig } from "../types/yamlConfig";
import { type AccountInfo, infrastructureApi, type ECSTaskInfo } from '../api/infrastructure';
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Alert, AlertDescription } from "./ui/alert";
import { Info, RefreshCw, Copy, Terminal, AlertCircle } from "lucide-react";

interface BackendSSHAccessProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
  accountInfo?: AccountInfo;
}

export function BackendSSHAccess({ config, onConfigChange, accountInfo }: BackendSSHAccessProps) {
  const [tasks, setTasks] = useState<ECSTaskInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copiedCommand, setCopiedCommand] = useState(false);

  const handleRemoteAccessToggle = (checked: boolean) => {
    onConfigChange({
      workload: {
        ...config.workload,
        backend_remote_access: checked
      }
    });
  };

  const fetchTasks = async () => {
    try {
      setLoading(true);
      setError(null);
      
      // Use the actual API endpoint: /api/ecs/tasks?env={environment}&service={serviceName}
      const response = await infrastructureApi.getServiceTasks(config.env, 'backend');
      setTasks(response.tasks || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch tasks');
      console.error('Failed to fetch ECS tasks:', err);
    } finally {
      setLoading(false);
    }
  };

  // Remove auto-fetch on toggle, only fetch when button is clicked

  const copyCommand = (command: string) => {
    navigator.clipboard.writeText(command);
    setCopiedCommand(true);
    setTimeout(() => setCopiedCommand(false), 2000);
  };

  const runningTasks = tasks.filter(task => task.lastStatus === 'RUNNING');
  const primaryTask = runningTasks[0]; // Use the first running task

  const generateSSHCommand = (taskArn?: string) => {
    const arn = taskArn || '{task-arn}';
    const clusterName = `${config.project}_cluster_${config.env}`;
    const containerName = `${config.project}_service_${config.env}`;
    const profile = accountInfo?.profile || '{profile-name}';
    return `AWS_PROFILE=${profile} aws ecs execute-command \\
--cluster ${clusterName} \\
--task ${arn} \\
--container ${containerName} \\
--interactive \\
--command "/bin/bash" \\
--region ${config.region}`;
  };

  return (
    <div className="space-y-6">
      {/* Remote Access Toggle */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Terminal className="w-5 h-5 text-blue-400" />
            Remote SSH Access
          </CardTitle>
          <CardDescription>
            Enable secure remote access to backend containers for debugging
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex-1">
              <Label htmlFor="remote_access">Enable Remote Access</Label>
              <p className="text-xs text-gray-500 mt-1">
                Provisions secure SSH access through ECS Exec
              </p>
            </div>
            <Switch
              id="remote_access"
              checked={config.workload?.backend_remote_access || false}
              onCheckedChange={handleRemoteAccessToggle}
              className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
            />
          </div>

          {config.workload?.backend_remote_access && (
            <div className="bg-green-900/20 border border-green-700 rounded-lg p-4">
              <div className="flex items-start gap-2">
                <Info className="w-4 h-4 text-green-400 mt-0.5" />
                <div className="flex-1">
                  <h4 className="text-sm font-medium text-green-400 mb-1">Remote Access Enabled</h4>
                  <p className="text-xs text-gray-300">
                    ECS Exec is enabled for secure SSH access to your backend containers.
                    No bastion host required.
                  </p>
                </div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* SSH Instructions */}
      {config.workload?.backend_remote_access && (
        <Card>
          <CardHeader>
            <CardTitle>SSH Connection</CardTitle>
            <CardDescription>
              Get running tasks and connect to your backend containers
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* SSH Command - Always Show */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <h4 className="text-sm font-medium text-gray-300">SSH Command</h4>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={fetchTasks}
                    disabled={loading}
                  >
                    <RefreshCw className={`w-3 h-3 mr-1 ${loading ? 'animate-spin' : ''}`} />
                    {loading ? 'Getting Tasks...' : 'Get Tasks'}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => copyCommand(generateSSHCommand(primaryTask?.taskArn))}
                    className="text-xs"
                  >
                    <Copy className="w-3 h-3 mr-1" />
                    {copiedCommand ? 'Copied!' : 'Copy'}
                  </Button>
                </div>
              </div>
              
              <div className="bg-gray-900 rounded-lg p-4 font-mono text-xs text-gray-300 overflow-x-auto">
                <pre>{generateSSHCommand(primaryTask?.taskArn)}</pre>
              </div>

              {!primaryTask && !loading && (
                <p className="text-xs text-gray-500 mt-2">
                  Click "Get Tasks" to fill in the actual task ARN
                </p>
              )}
              
              {primaryTask && (
                <p className="text-xs text-green-400 mt-2">
                  ✓ Using task ARN from running task
                </p>
              )}
            </div>

            {error && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            {/* Running Tasks Info - Show only after tasks are fetched */}
            {runningTasks.length > 0 && (
              <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
                <div className="flex items-center justify-between mb-3">
                  <h4 className="text-sm font-medium text-blue-400">Running Tasks</h4>
                  <span className="text-xs text-gray-400">{runningTasks.length} active</span>
                </div>
                <div className="space-y-3">
                  {runningTasks.slice(0, 3).map((task, index) => (
                    <div key={task.taskArn} className="bg-gray-800 rounded p-3">
                      <div className="flex items-start justify-between mb-2">
                        <span className="text-xs font-medium text-gray-300">Task {index + 1}</span>
                        <div className="flex items-center gap-2">
                          <span className="text-xs px-2 py-0.5 bg-green-800 text-green-300 rounded">
                            {task.lastStatus}
                          </span>
                          {task.healthStatus && (
                            <span className="text-xs px-2 py-0.5 bg-blue-800 text-blue-300 rounded">
                              {task.healthStatus}
                            </span>
                          )}
                        </div>
                      </div>
                      <div className="space-y-1 text-xs text-gray-400">
                        <div className="font-mono text-gray-300 break-all">
                          ARN: {task.taskArn.split('/').pop()}
                        </div>
                        <div className="flex items-center gap-4">
                          <span>CPU: {task.cpu}</span>
                          <span>Memory: {task.memory}</span>
                          <span>AZ: {task.availabilityZone}</span>
                        </div>
                        <div>Started: {new Date(task.startedAt || task.createdAt).toLocaleString()}</div>
                      </div>
                    </div>
                  ))}
                  {runningTasks.length > 3 && (
                    <p className="text-xs text-gray-500 text-center">+ {runningTasks.length - 3} more tasks</p>
                  )}
                </div>
              </div>
            )}

            {/* Prerequisites - Always Show */}
            <div className="bg-gray-800 rounded-lg p-4">
              <h4 className="text-sm font-medium text-gray-300 mb-2">Prerequisites</h4>
              <ul className="text-xs text-gray-400 space-y-1">
                <li>• AWS CLI configured with appropriate permissions</li>
                <li>• Session Manager plugin: <code className="text-gray-300">aws ssm install-plugin</code></li>
                <li>• Your container must include <code className="text-gray-300">/bin/bash</code></li>
              </ul>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Disabled State */}
      {!config.workload?.backend_remote_access && (
        <Card>
          <CardHeader>
            <CardTitle>Getting Started</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-center py-8">
              <Terminal className="w-12 h-12 text-gray-600 mx-auto mb-4" />
              <h3 className="text-sm font-medium text-gray-300 mb-2">Remote Access Disabled</h3>
              <p className="text-xs text-gray-500 mb-4">
                Enable remote access to SSH into your backend containers for debugging and troubleshooting.
              </p>
              <Button
                onClick={() => handleRemoteAccessToggle(true)}
                className="bg-blue-600 hover:bg-blue-700"
              >
                Enable Remote Access
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
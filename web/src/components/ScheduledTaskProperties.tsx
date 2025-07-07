import React, { useState } from "react";
import { YamlInfrastructureConfig } from "../types/yamlConfig";
import { type AccountInfo } from '../api/infrastructure';
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Separator } from "./ui/separator";
import { Info, Clock, Calendar } from "lucide-react";
import { ComponentNode } from "../types";
import { ScheduleExpressionBuilder } from "./ScheduleExpressionBuilder";

interface ScheduledTaskPropertiesProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
  accountInfo?: AccountInfo;
  node: ComponentNode;
}

export function ScheduledTaskProperties({ config, onConfigChange, accountInfo, node }: ScheduledTaskPropertiesProps) {
  // Extract task name from node id (e.g., "scheduled-daily-report" -> "daily-report")
  const taskName = node.id.replace('scheduled-', '');
  
  // Find the current task in config
  const currentTask = config.scheduled_tasks?.find(task => task.name === taskName);
  
  // Generate the ECR repository name for this specific task
  const taskEcrRepoName = `${config.project}_task_${taskName}`;
  
  // Use accountInfo if available, otherwise fall back to config values
  const accountId = accountInfo?.accountId || config.ecr_account_id;
  const region = config.ecr_account_region || config.region;
  
  // Always show the actual account ID when available
  const defaultTaskEcrUri = `${accountId || '<YOUR_ACCOUNT_ID>'}.dkr.ecr.${region}.amazonaws.com/${taskEcrRepoName}`;
  
  const isDev = config.env === 'dev' || !config.is_prod;

  const handleTaskChange = (updates: Partial<typeof currentTask>) => {
    if (!currentTask) {
      // If task doesn't exist, create it
      const newTask = {
        name: taskName,
        schedule: "rate(1 day)",
        ...updates
      };
      onConfigChange({
        scheduled_tasks: [...(config.scheduled_tasks || []), newTask]
      });
    } else {
      // Update existing task
      const updatedTasks = config.scheduled_tasks?.map(task => 
        task.name === taskName ? { ...task, ...updates } : task
      ) || [];
      
      onConfigChange({
        scheduled_tasks: updatedTasks
      });
    }
  };

  // Use currentTask if it exists, otherwise use defaults
  const task = currentTask || {
    name: taskName,
    schedule: "rate(1 day)",
    docker_image: "",
    container_command: "",
    allow_public_access: false
  };

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>Scheduled Task Configuration</CardTitle>
        <CardDescription>
          Configure settings for scheduled task: {taskName}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label>Schedule Expression</Label>
          <ScheduleExpressionBuilder
            value={task.schedule || "rate(1 day)"}
            onChange={(schedule) => handleTaskChange({ schedule })}
          />
        </div>

        <Separator />

        <div className="space-y-2">
          <Label htmlFor="docker_image">Docker Image (Optional)</Label>
          <Input
            id="docker_image"
            value={task.docker_image || ""}
            onChange={(e) => handleTaskChange({ docker_image: e.target.value })}
            placeholder={`${defaultTaskEcrUri}:latest`}
            className="bg-gray-800 border-gray-600 text-white font-mono text-sm"
          />
          <div className="mt-2 p-3 bg-blue-900/20 border border-blue-700 rounded-lg">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-blue-400 mt-0.5 flex-shrink-0" />
              <div className="flex-1">
                <p className="text-xs text-gray-300">
                  <strong className="text-blue-400">Task ECR Repository:</strong>
                </p>
                <p className="text-xs font-mono text-gray-400 mt-1 break-all">
                  {defaultTaskEcrUri}:latest
                </p>
                <div className="mt-2 space-y-1">
                  <p className="text-xs text-gray-500">
                    <strong>Repository Name:</strong> <code className="text-gray-400">{taskEcrRepoName}</code>
                  </p>
                  {isDev ? (
                    <p className="text-xs text-green-400">
                      ✓ Dev environment: ECR repository will be created automatically
                    </p>
                  ) : (
                    <p className="text-xs text-yellow-400">
                      ⚠ Production: You must create the ECR repository and provide ecr_url
                    </p>
                  )}
                </div>
                <div className="mt-2 pt-2 border-t border-gray-700">
                  <p className="text-xs text-gray-300 font-semibold mb-1">Docker Image Resolution Priority:</p>
                  <ol className="text-xs text-gray-500 space-y-0.5 list-decimal list-inside">
                    <li>If specified here → Use custom image</li>
                    <li>In dev → Use <code>{taskEcrRepoName}:latest</code></li>
                    <li>In prod → Use <code>ecr_url:latest</code></li>
                  </ol>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="container_command">Container Command Override</Label>
          <Input
            id="container_command"
            value={task.container_command || ""}
            onChange={(e) => handleTaskChange({ container_command: e.target.value })}
            placeholder='["npm", "run", "report"]'
            className="bg-gray-800 border-gray-600 text-white font-mono"
          />
          <p className="text-xs text-gray-500">
            Override container startup command (JSON array as string)
          </p>
        </div>

        <Separator />

        <div className="flex items-center justify-between">
          <div className="flex-1">
            <Label htmlFor="allow_public_access">Allow Public Access</Label>
            <p className="text-xs text-gray-500 mt-1">Assign public IP to the task</p>
          </div>
          <Switch
            id="allow_public_access"
            checked={task.allow_public_access || false}
            onCheckedChange={(checked) => handleTaskChange({ allow_public_access: checked })}
            className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
          />
        </div>

      </CardContent>
    </Card>
  );
}
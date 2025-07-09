import { YamlInfrastructureConfig } from "../types/yamlConfig";
import { type AccountInfo } from '../api/infrastructure';
import { ComponentNode } from '../types';
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Separator } from "./ui/separator";
import { Alert, AlertDescription } from "./ui/alert";
import { Info, AlertTriangle } from "lucide-react";

interface ServicePropertiesProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
  accountInfo?: AccountInfo;
  node: ComponentNode;
}

export function ServiceProperties({ config, onConfigChange, accountInfo, node }: ServicePropertiesProps) {
  // Extract service name from node id
  const serviceName = node.id.replace('service-', '');
  
  // Find the service configuration
  const serviceConfig = config.services?.find(service => service.name === serviceName);
  
  // Generate the ECR repository name based on config
  const ecrRepoName = `${config.project}_${serviceName}`;
  
  // Use accountInfo if available, otherwise fall back to config values
  const accountId = accountInfo?.accountId || config.ecr_account_id;
  const region = config.ecr_account_region || config.region;
  
  // ECR URI - note that ECR repos for additional services are only created in dev environment
  const ecrUri = `${accountId || '<YOUR_ACCOUNT_ID>'}.dkr.ecr.${region}.amazonaws.com/${ecrRepoName}`;

  const handleServiceChange = (updates: Partial<NonNullable<YamlInfrastructureConfig['services']>[0]>) => {
    if (!config.services) return;
    
    const updatedServices = config.services.map(service => 
      service.name === serviceName 
        ? { ...service, ...updates }
        : service
    );
    
    onConfigChange({ services: updatedServices });
  };

  if (!serviceConfig) {
    return (
      <Alert className="border-red-600">
        <AlertTriangle className="h-4 w-4 text-red-600" />
        <AlertDescription>
          Service "{serviceName}" not found in configuration.
        </AlertDescription>
      </Alert>
    );
  }

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>{serviceName} Service Configuration</CardTitle>
        <CardDescription>
          Configure your service settings
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Essential Container Toggle - at the top */}
        <div className="flex items-center justify-between">
          <div className="flex-1">
            <Label htmlFor="essential">Essential Container</Label>
            <p className="text-xs text-gray-500 mt-1">If this container stops, stop all other containers</p>
          </div>
          <Switch
            id="essential"
            checked={serviceConfig.essential !== false} // default true
            onCheckedChange={(checked) => handleServiceChange({ essential: checked })}
            className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
          />
        </div>

        <Separator />

        <div className="space-y-2">
          <Label htmlFor="docker_image">External Docker Image</Label>
          <Input
            id="docker_image"
            value={serviceConfig.docker_image || ""}
            onChange={(e) => handleServiceChange({ docker_image: e.target.value })}
            placeholder="docker.io/myapp:latest"
            className="bg-gray-800 border-gray-600 text-white font-mono"
          />
          <p className="text-xs text-gray-500">
            Optional: Use external Docker image instead of the ECR repository
          </p>
          
          {/* ECR Repository Info */}
          <div className="mt-2 p-3 bg-blue-900/20 border border-blue-700 rounded-lg">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-blue-400 mt-0.5 flex-shrink-0" />
              <div className="flex-1">
                <p className="text-xs text-gray-300">
                  <strong className="text-blue-400">Default ECR Repository (Dev only):</strong>
                </p>
                <p className="text-xs font-mono text-gray-400 mt-1 break-all">
                  {ecrUri}
                </p>
                <p className="text-xs text-gray-500 mt-2">
                  ECR repositories for services are only created in development environment.
                  In production, you must use an external Docker image.
                </p>
              </div>
            </div>
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="container_command">Container Command</Label>
          <Input
            id="container_command"
            value={
              Array.isArray(serviceConfig.container_command)
                ? serviceConfig.container_command.join(", ")
                : serviceConfig.container_command || ""
            }
            onChange={(e) => {
              const commands = e.target.value.split(",").map(cmd => cmd.trim()).filter(cmd => cmd);
              handleServiceChange({ container_command: commands.length > 0 ? commands : undefined });
            }}
            placeholder='npm, start'
            className="bg-gray-800 border-gray-600 text-white font-mono"
          />
          <p className="text-xs text-gray-500">Override container startup command (comma-separated)</p>
        </div>

        <Separator />

        <div className="space-y-2">
          <Label htmlFor="container_port">Container Port</Label>
          <Input
            id="container_port"
            type="number"
            value={serviceConfig.container_port || 3000}
            onChange={(e) => handleServiceChange({ container_port: parseInt(e.target.value) || 3000 })}
            placeholder="3000"
            className="bg-gray-800 border-gray-600 text-white"
          />
          <p className="text-xs text-gray-500">Port your application listens on (default: 3000)</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="host_port">Host Port</Label>
          <Input
            id="host_port"
            type="number"
            value={serviceConfig.host_port || 3000}
            onChange={(e) => handleServiceChange({ host_port: parseInt(e.target.value) || 3000 })}
            placeholder="3000"
            className="bg-gray-800 border-gray-600 text-white"
          />
          <p className="text-xs text-gray-500">Host port mapping (default: 3000)</p>
        </div>
      </CardContent>
    </Card>
  );
}
import { useEffect, useState } from "react";
import { InfrastructureConfig } from "../types/config";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Separator } from "./ui/separator";

interface BackendServicePropertiesProps {
  config: InfrastructureConfig;
  onConfigChange: (config: Partial<InfrastructureConfig>) => void;
}

export function BackendServiceProperties({ config, onConfigChange }: BackendServicePropertiesProps) {
  const [localConfig, setLocalConfig] = useState<Partial<InfrastructureConfig>>({
    health_endpoint: config.health_endpoint,
    backend_external_docker_image: config.backend_external_docker_image,
    backend_container_command: config.backend_container_command,
    image_bucket_postfix: config.image_bucket_postfix,
    bucket_public: config.bucket_public,
    backend_image_port: config.backend_image_port,
    xray_enabled: config.xray_enabled,
  });

  useEffect(() => {
    setLocalConfig({
      health_endpoint: config.health_endpoint,
      backend_external_docker_image: config.backend_external_docker_image,
      backend_container_command: config.backend_container_command,
      image_bucket_postfix: config.image_bucket_postfix,
      bucket_public: config.bucket_public,
      backend_image_port: config.backend_image_port,
      xray_enabled: config.xray_enabled,
    });
  }, [config]);

  const handleChange = (field: keyof InfrastructureConfig, value: any) => {
    const updatedConfig = { ...localConfig, [field]: value };
    setLocalConfig(updatedConfig);
    onConfigChange(updatedConfig);
  };

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>Backend Service Configuration</CardTitle>
        <CardDescription>
          Configure your backend service settings
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="health_endpoint">Health Endpoint</Label>
          <Input
            id="health_endpoint"
            value={localConfig.health_endpoint || ""}
            onChange={(e) => handleChange("health_endpoint", e.target.value)}
            placeholder="/health"
          />
          <p className="text-xs text-muted-foreground">API endpoint for health checks</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="backend_external_docker_image">External Docker Image</Label>
          <Input
            id="backend_external_docker_image"
            value={localConfig.backend_external_docker_image || ""}
            onChange={(e) => handleChange("backend_external_docker_image", e.target.value)}
            placeholder="docker.io/myapp:latest"
          />
          <p className="text-xs text-muted-foreground">Optional: Use external Docker image instead of building</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="backend_container_command">Container Command</Label>
          <Input
            id="backend_container_command"
            value={localConfig.backend_container_command || ""}
            onChange={(e) => handleChange("backend_container_command", e.target.value)}
            placeholder='["npm", "start"]'
          />
          <p className="text-xs text-muted-foreground">Override container startup command</p>
        </div>

        <Separator />

        <div className="space-y-2">
          <Label htmlFor="image_bucket_postfix">Image Bucket Postfix</Label>
          <Input
            id="image_bucket_postfix"
            value={localConfig.image_bucket_postfix || ""}
            onChange={(e) => handleChange("image_bucket_postfix", e.target.value)}
            placeholder="images"
          />
          <p className="text-xs text-muted-foreground">S3 bucket name postfix for storing images</p>
        </div>

        <div className="flex items-center space-x-2">
          <Switch
            id="bucket_public"
            checked={localConfig.bucket_public || false}
            onCheckedChange={(checked) => handleChange("bucket_public", checked)}
          />
          <Label htmlFor="bucket_public">Public Bucket</Label>
        </div>

        <Separator />

        <div className="space-y-2">
          <Label htmlFor="backend_image_port">Container Port</Label>
          <Input
            id="backend_image_port"
            type="number"
            value={localConfig.backend_image_port || 3000}
            onChange={(e) => handleChange("backend_image_port", parseInt(e.target.value) || 3000)}
            placeholder="3000"
          />
          <p className="text-xs text-muted-foreground">Port your application listens on</p>
        </div>

        <div className="flex items-center space-x-2">
          <Switch
            id="xray_enabled"
            checked={localConfig.xray_enabled !== false}
            onCheckedChange={(checked) => handleChange("xray_enabled", checked)}
          />
          <Label htmlFor="xray_enabled">Enable AWS X-Ray Tracing</Label>
        </div>
      </CardContent>
    </Card>
  );
}
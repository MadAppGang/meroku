import { useEffect, useState } from "react";
import { InfrastructureConfig, AWS_REGIONS } from "../types/config";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";
import { Switch } from "./ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";

interface ECSNodePropertiesProps {
  config: InfrastructureConfig;
  onConfigChange: (config: Partial<InfrastructureConfig>) => void;
}

export function ECSNodeProperties({ config, onConfigChange }: ECSNodePropertiesProps) {
  const [localConfig, setLocalConfig] = useState<Partial<InfrastructureConfig>>({
    project: config.project,
    env: config.env,
    is_prod: config.is_prod,
    region: config.region,
  });

  useEffect(() => {
    setLocalConfig({
      project: config.project,
      env: config.env,
      is_prod: config.is_prod,
      region: config.region,
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
        <CardTitle>ECS Cluster Configuration</CardTitle>
        <CardDescription>
          Core settings for your ECS cluster
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="project">Project Name</Label>
          <Input
            id="project"
            value={localConfig.project || ""}
            onChange={(e) => handleChange("project", e.target.value)}
            placeholder="Enter project name"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="env">Environment</Label>
          <Input
            id="env"
            value={localConfig.env || ""}
            disabled
            className="bg-muted cursor-not-allowed"
          />
          <p className="text-xs text-muted-foreground">Environment is read-only</p>
        </div>

        <div className="flex items-center space-x-2">
          <Switch
            id="is_prod"
            checked={localConfig.is_prod || false}
            onCheckedChange={(checked) => handleChange("is_prod", checked)}
          />
          <Label htmlFor="is_prod">Production Environment</Label>
        </div>

        <div className="space-y-2">
          <Label htmlFor="region">AWS Region</Label>
          <Select
            value={localConfig.region || ""}
            onValueChange={(value) => handleChange("region", value)}
          >
            <SelectTrigger id="region">
              <SelectValue placeholder="Select AWS region" />
            </SelectTrigger>
            <SelectContent>
              {AWS_REGIONS.map((region) => (
                <SelectItem key={region.value} value={region.value}>
                  {region.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardContent>
    </Card>
  );
}
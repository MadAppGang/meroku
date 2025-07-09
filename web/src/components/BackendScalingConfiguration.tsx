import { useState, useEffect } from 'react';
import type { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Info, AlertCircle, Cpu, Users, Activity } from 'lucide-react';
import { Label } from './ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { Input } from './ui/input';
import { Switch } from './ui/switch';
import { Slider } from './ui/slider';
import { Checkbox } from './ui/checkbox';

interface BackendScalingConfigurationProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
  isService?: boolean;
  serviceName?: string;
}

// CPU to Memory mapping based on Fargate requirements
const cpuMemoryMap: { [key: string]: string[] } = {
  '256': ['512', '1024', '2048'],
  '512': ['1024', '2048', '3072', '4096'],
  '1024': ['2048', '3072', '4096', '5120', '6144', '7168', '8192'],
  '2048': ['4096', '5120', '6144', '7168', '8192', '9216', '10240', '11264', '12288', '13312', '14336', '15360', '16384'],
  '4096': ['8192', '9216', '10240', '11264', '12288', '13312', '14336', '15360', '16384', '17408', '18432', '19456', '20480', '21504', '22528', '23552', '24576', '25600', '26624', '27648', '28672', '29696', '30720']
};

export function BackendScalingConfiguration({ config, onConfigChange, isService = false, serviceName }: BackendScalingConfigurationProps) {
  // Get service config if this is for a service
  const serviceConfig = isService && serviceName ? config.services?.find(s => s.name === serviceName) : null;
  
  const [cpu, setCpu] = useState(isService && serviceConfig ? (serviceConfig.cpu?.toString() || '256') : (config.workload?.backend_cpu || '256'));
  const [memory, setMemory] = useState(isService && serviceConfig ? (serviceConfig.memory?.toString() || '512') : (config.workload?.backend_memory || '512'));
  const [desiredCount, setDesiredCount] = useState(isService && serviceConfig ? (serviceConfig.desired_count || 1) : (config.workload?.backend_desired_count || 1));
  const [autoscalingEnabled, setAutoscalingEnabled] = useState(isService ? false : (config.workload?.backend_autoscaling_enabled || false));
  const [minCapacity, setMinCapacity] = useState(config.workload?.backend_autoscaling_min_capacity || 1);
  const [maxCapacity, setMaxCapacity] = useState(config.workload?.backend_autoscaling_max_capacity || 10);
  const [cpuTarget, setCpuTarget] = useState(70);
  const [memoryTarget, setMemoryTarget] = useState(80);
  const [requestBasedScaling, setRequestBasedScaling] = useState(false);
  const [requestsPerTarget, setRequestsPerTarget] = useState(1000);

  // Adjust memory when CPU changes
  useEffect(() => {
    const availableMemory = cpuMemoryMap[cpu];
    if (availableMemory && !availableMemory.includes(memory)) {
      setMemory(availableMemory[0]);
    }
  }, [cpu, memory]);

  const handleWorkloadChange = (updates: Partial<YamlInfrastructureConfig['workload']>) => {
    if (isService && serviceName) {
      // Update service configuration
      const updatedServices = config.services?.map(service => 
        service.name === serviceName 
          ? { 
              ...service, 
              cpu: updates?.backend_cpu ? Number.parseInt(updates.backend_cpu) : service.cpu,
              memory: updates?.backend_memory ? Number.parseInt(updates.backend_memory) : service.memory,
              desired_count: updates?.backend_desired_count || service.desired_count
            }
          : service
      ) || [];
      
      onConfigChange({ services: updatedServices });
    } else {
      // Update backend configuration
      onConfigChange({
        workload: {
          ...config.workload,
          ...updates
        }
      });
    }
  };

  // X-Ray no longer affects resource allocation

  return (
    <div className="space-y-6">
      {/* Resource Configuration */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Cpu className="w-5 h-5 text-blue-400" />
            Resource Configuration
          </CardTitle>
          <CardDescription>
            Configure CPU and memory resources for each instance
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label htmlFor="cpu-units">CPU Units</Label>
              <Select
                value={cpu}
                onValueChange={(value: string) => {
                  setCpu(value);
                  handleWorkloadChange({ backend_cpu: value });
                }}
              >
                <SelectTrigger id="cpu-units" className="mt-1 bg-gray-800 border-gray-600">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="256">256 (0.25 vCPU)</SelectItem>
                  <SelectItem value="512">512 (0.5 vCPU)</SelectItem>
                  <SelectItem value="1024">1024 (1 vCPU)</SelectItem>
                  <SelectItem value="2048">2048 (2 vCPU)</SelectItem>
                  <SelectItem value="4096">4096 (4 vCPU)</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div>
              <Label htmlFor="memory">Memory (MB)</Label>
              <Select
                value={memory}
                onValueChange={(value: string) => {
                  setMemory(value);
                  handleWorkloadChange({ backend_memory: value });
                }}
              >
                <SelectTrigger id="memory" className="mt-1 bg-gray-800 border-gray-600">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {cpuMemoryMap[cpu]?.map(mem => (
                    <SelectItem key={mem} value={mem}>
                      {mem} MB ({(Number.parseInt(mem) / 1024).toFixed(1)} GB)
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div>
            <Label htmlFor="base-count">Base Instance Count</Label>
            <Input
              id="base-count"
              type="number"
              min="1"
              max="100"
              value={desiredCount}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                const value = Number.parseInt(e.target.value) || 1;
                setDesiredCount(value);
                handleWorkloadChange({ backend_desired_count: value });
              }}
              className="mt-1 bg-gray-800 border-gray-600 text-white"
            />
            <p className="text-xs text-gray-500 mt-1">
              Number of instances to run when autoscaling is disabled
            </p>
          </div>


          <div className="bg-gray-800 rounded-lg p-4">
            <h4 className="text-sm font-medium text-gray-300 mb-2">Resource Guidelines</h4>
            <div className="space-y-2 text-xs text-gray-400">
              <p>• <strong>256 CPU:</strong> Light workloads, simple APIs</p>
              <p>• <strong>512 CPU:</strong> Standard web applications</p>
              <p>• <strong>1024 CPU:</strong> CPU-intensive processing</p>
              <p>• <strong>2048+ CPU:</strong> High-performance applications</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Autoscaling Configuration - Only show for backend, not services */}
      {!isService && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="w-5 h-5 text-green-400" />
              Autoscaling Configuration
            </CardTitle>
            <CardDescription>
              Configure automatic scaling based on metrics
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="flex-1">
                <Label htmlFor="enable-autoscaling">Enable Autoscaling</Label>
                <p className="text-xs text-gray-500 mt-1">
                  Automatically adjust the number of instances based on load
                </p>
              </div>
              <Switch
                id="enable-autoscaling"
                checked={autoscalingEnabled}
                onCheckedChange={(checked) => {
                  setAutoscalingEnabled(checked);
                  handleWorkloadChange({ backend_autoscaling_enabled: checked });
                }}
                className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
              />
            </div>

            {autoscalingEnabled && (
            <>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Label htmlFor="min-instances">Minimum Instances</Label>
                  <Input
                    id="min-instances"
                    type="number"
                    min="1"
                    max={maxCapacity}
                    value={minCapacity}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                      const value = Number.parseInt(e.target.value) || 1;
                      setMinCapacity(value);
                      handleWorkloadChange({ backend_autoscaling_min_capacity: value });
                    }}
                    className="mt-1 bg-gray-800 border-gray-600 text-white"
                  />
                </div>

                <div>
                  <Label htmlFor="max-instances">Maximum Instances</Label>
                  <Input
                    id="max-instances"
                    type="number"
                    min={minCapacity}
                    max="100"
                    value={maxCapacity}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                      const value = Number.parseInt(e.target.value) || 1;
                      setMaxCapacity(value);
                      handleWorkloadChange({ backend_autoscaling_max_capacity: value });
                    }}
                    className="mt-1 bg-gray-800 border-gray-600 text-white"
                  />
                </div>
              </div>

              <div className="space-y-4">
                <h4 className="text-sm font-medium text-gray-300">Scaling Triggers</h4>
                
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <Label>CPU Target</Label>
                    <span className="text-sm text-gray-400">{cpuTarget}%</span>
                  </div>
                  <Slider
                    value={[cpuTarget]}
                    onValueChange={([value]: number[]) => {
                      setCpuTarget(value);
                      // Note: cpu_target is not persisted in config
                    }}
                    min={0}
                    max={100}
                    step={5}
                    className="mt-1"
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    Scale up when average CPU utilization exceeds this threshold
                  </p>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-2">
                    <Label>Memory Target</Label>
                    <span className="text-sm text-gray-400">{memoryTarget}%</span>
                  </div>
                  <Slider
                    value={[memoryTarget]}
                    onValueChange={([value]: number[]) => {
                      setMemoryTarget(value);
                      // Note: memory_target is not persisted in config
                    }}
                    min={0}
                    max={100}
                    step={5}
                    className="mt-1"
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    Scale up when average memory utilization exceeds this threshold
                  </p>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <Checkbox
                      id="request-scaling"
                      checked={requestBasedScaling}
                      onCheckedChange={(checked) => {
                        setRequestBasedScaling(checked as boolean);
                        // Note: request_based is not persisted in config
                      }}
                    />
                    <Label htmlFor="request-scaling" className="text-sm font-normal cursor-pointer">
                      Enable Request-based Scaling (requires ALB)
                    </Label>
                  </div>
                  
                  {requestBasedScaling && (
                    <div className="ml-6">
                      <Label htmlFor="requests-per-instance">Requests per Instance</Label>
                      <Input
                        id="requests-per-instance"
                        type="number"
                        min="100"
                        max="10000"
                        value={requestsPerTarget}
                        onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                          const value = Number.parseInt(e.target.value) || 1000;
                          setRequestsPerTarget(value);
                          // Note: requests_per_target is not persisted in config
                        }}
                        className="mt-1 bg-gray-800 border-gray-600 text-white"
                      />
                      <p className="text-xs text-gray-500 mt-1">
                        Target number of requests each instance should handle
                      </p>
                    </div>
                  )}
                </div>
              </div>

              <div className="bg-green-900/20 border border-green-700 rounded-lg p-4">
                <div className="flex items-start gap-2">
                  <Info className="w-4 h-4 text-green-400 mt-0.5" />
                  <div className="flex-1">
                    <h4 className="text-sm font-medium text-green-400 mb-1">Scaling Behavior</h4>
                    <p className="text-xs text-gray-300">
                      The service will scale between {minCapacity} and {maxCapacity} instances based on the configured metrics. 
                      Scaling decisions are made every 60 seconds.
                    </p>
                  </div>
                </div>
              </div>
            </>
          )}

            {!autoscalingEnabled && (
              <div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
                <div className="flex items-start gap-2">
                  <AlertCircle className="w-4 h-4 text-yellow-400 mt-0.5" />
                  <div className="flex-1">
                    <h4 className="text-sm font-medium text-yellow-400 mb-1">Manual Scaling</h4>
                    <p className="text-xs text-gray-300">
                      With autoscaling disabled, your service will always run exactly {desiredCount} instance{desiredCount > 1 ? 's' : ''}. 
                      You'll need to manually adjust this value to handle load changes.
                    </p>
                  </div>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}


      {/* Resource Summary */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Users className="w-5 h-5 text-purple-400" />
            Resource Summary
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div className="bg-gray-800 rounded-lg p-3">
                <p className="text-xs text-gray-400 mb-1">Per Instance</p>
                <p className="text-gray-300">
                  {cpu} CPU / {memory} MB
                </p>
              </div>
              
              <div className="bg-gray-800 rounded-lg p-3">
                <p className="text-xs text-gray-400 mb-1">Instance Range</p>
                <p className="text-gray-300">
                  {autoscalingEnabled ? `${minCapacity} - ${maxCapacity}` : desiredCount} instances
                </p>
              </div>
            </div>

            <div className="bg-gray-800 rounded-lg p-3">
              <p className="text-xs text-gray-400 mb-1">Maximum Resources</p>
              <p className="text-sm text-gray-300">
                {Number.parseInt(cpu) * (autoscalingEnabled ? maxCapacity : desiredCount)} CPU units / 
                {' '}{(Number.parseInt(memory) * (autoscalingEnabled ? maxCapacity : desiredCount) / 1024).toFixed(1)} GB memory
              </p>
            </div>

            <div className="text-xs text-gray-500">
              <p>• Fargate pricing is based on CPU and memory allocated</p>
              <p>• You only pay for what you use (per-second billing)</p>
              <p>• Data transfer and other AWS services incur additional charges</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
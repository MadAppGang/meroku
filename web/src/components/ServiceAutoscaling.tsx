import React, { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Slider } from './ui/slider';
import { Switch } from './ui/switch';
import { Label } from './ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { Alert, AlertDescription } from './ui/alert';
import { Tabs, TabsContent, TabsList, TabsTrigger } from './ui/tabs';
import { 
  Activity, 
  AlertCircle, 
  TrendingUp, 
  TrendingDown,
  Cpu,
  MemoryStick,
  Users,
  DollarSign,
  Clock,
  RefreshCw
} from 'lucide-react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer
} from 'recharts';
import { infrastructureApi, ServiceAutoscalingInfo, ServiceScalingHistory, ServiceMetrics } from '../api/infrastructure';
import { format, parseISO } from 'date-fns';

interface ServiceAutoscalingProps {
  environment: string;
  serviceName: string;
}

// CPU/Memory valid combinations for Fargate
const CPU_MEMORY_COMBINATIONS: Record<number, number[]> = {
  256: [512, 1024, 2048],
  512: [1024, 2048, 3072, 4096],
  1024: [2048, 3072, 4096, 5120, 6144, 7168, 8192],
  2048: [4096, 5120, 6144, 7168, 8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384],
  4096: [8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384, 17408, 18432, 19456, 20480, 21504, 22528, 23552, 24576, 25600, 26624, 27648, 28672, 29696, 30720],
};

export function ServiceAutoscaling({ environment, serviceName }: ServiceAutoscalingProps) {
  const [autoscalingInfo, setAutoscalingInfo] = useState<ServiceAutoscalingInfo | null>(null);
  const [scalingHistory, setScalingHistory] = useState<ServiceScalingHistory | null>(null);
  const [metrics, setMetrics] = useState<ServiceMetrics | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  // Form state
  const [formData, setFormData] = useState({
    enabled: false,
    cpu: 256,
    memory: 512,
    desiredCount: 1,
    minCapacity: 1,
    maxCapacity: 10,
    targetCPU: 70,
    targetMemory: 80,
  });

  // Fetch all data
  const fetchData = async () => {
    try {
      setRefreshing(true);
      const [info, history, metricsData] = await Promise.all([
        infrastructureApi.getServiceAutoscaling(environment, serviceName),
        infrastructureApi.getServiceScalingHistory(environment, serviceName, 24),
        infrastructureApi.getServiceMetrics(environment, serviceName),
      ]);

      setAutoscalingInfo(info);
      setScalingHistory(history);
      setMetrics(metricsData);

      // Update form with current values
      setFormData({
        enabled: info.enabled,
        cpu: info.cpu,
        memory: info.memory,
        desiredCount: info.currentDesiredCount,
        minCapacity: info.minCapacity,
        maxCapacity: info.maxCapacity,
        targetCPU: info.targetCPU,
        targetMemory: info.targetMemory,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch autoscaling data');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    fetchData();
    // Refresh every 30 seconds
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, [environment, serviceName]);

  // Get valid memory options for selected CPU
  const getValidMemoryOptions = (cpu: number) => {
    return CPU_MEMORY_COMBINATIONS[cpu] || [];
  };

  // Format timestamp for display
  const formatTimestamp = (timestamp: string) => {
    return format(parseISO(timestamp), 'MMM dd, HH:mm');
  };

  // Calculate estimated monthly cost (simplified)
  const calculateMonthlyCost = (cpu: number, memory: number, instanceCount: number) => {
    // Fargate pricing per vCPU hour: $0.04048
    // Fargate pricing per GB hour: $0.004445
    const cpuCost = (cpu / 1024) * 0.04048 * 24 * 30 * instanceCount;
    const memoryCost = (memory / 1024) * 0.004445 * 24 * 30 * instanceCount;
    return (cpuCost + memoryCost).toFixed(2);
  };

  // Prepare chart data
  const prepareChartData = () => {
    if (!metrics) return [];

    const cpuMap = new Map(metrics.metrics.cpu.map(m => [m.timestamp, m.value]));
    const memoryMap = new Map(metrics.metrics.memory.map(m => [m.timestamp, m.value]));
    const taskMap = new Map(metrics.metrics.taskCount.map(m => [m.timestamp, m.value]));

    const timestamps = [...new Set([
      ...metrics.metrics.cpu.map(m => m.timestamp),
      ...metrics.metrics.memory.map(m => m.timestamp),
      ...metrics.metrics.taskCount.map(m => m.timestamp),
    ])].sort();

    return timestamps.map(timestamp => ({
      time: formatTimestamp(timestamp),
      cpu: cpuMap.get(timestamp) || 0,
      memory: memoryMap.get(timestamp) || 0,
      tasks: taskMap.get(timestamp) || 0,
    }));
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="w-8 h-8 animate-spin text-gray-400" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert className="m-4">
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    );
  }

  const chartData = prepareChartData();

  return (
    <div className="space-y-6 p-4">
      {/* Current Status Card */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Autoscaling Status</CardTitle>
            <CardDescription>Current configuration and metrics</CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={fetchData}
            disabled={refreshing}
          >
            <RefreshCw className={`w-4 h-4 mr-2 ${refreshing ? 'animate-spin' : ''}`} />
            Refresh
          </Button>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <div className="space-y-2">
              <Label className="text-sm text-gray-500">Status</Label>
              <div className="flex items-center gap-2">
                <Badge variant={autoscalingInfo?.enabled ? 'default' : 'secondary'}>
                  {autoscalingInfo?.enabled ? 'Enabled' : 'Disabled'}
                </Badge>
              </div>
            </div>
            <div className="space-y-2">
              <Label className="text-sm text-gray-500">Current Instances</Label>
              <div className="text-2xl font-bold">{autoscalingInfo?.currentDesiredCount || 0}</div>
            </div>
            <div className="space-y-2">
              <Label className="text-sm text-gray-500">CPU Utilization</Label>
              <div className="flex items-center gap-2">
                <Cpu className="w-4 h-4 text-blue-500" />
                <span className="text-lg font-semibold">
                  {autoscalingInfo?.currentCPUUtilization?.toFixed(1) || '0'}%
                </span>
              </div>
            </div>
            <div className="space-y-2">
              <Label className="text-sm text-gray-500">Memory Utilization</Label>
              <div className="flex items-center gap-2">
                <MemoryStick className="w-4 h-4 text-green-500" />
                <span className="text-lg font-semibold">
                  {autoscalingInfo?.currentMemoryUtilization?.toFixed(1) || '0'}%
                </span>
              </div>
            </div>
          </div>

          {autoscalingInfo?.lastScalingActivity && (
            <Alert className="mt-4">
              <Activity className="h-4 w-4" />
              <AlertDescription>
                <strong>Last scaling activity:</strong> {autoscalingInfo.lastScalingActivity.description}
                <br />
                <span className="text-sm text-gray-500">
                  {formatTimestamp(autoscalingInfo.lastScalingActivity.time)}
                </span>
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      {/* Configuration Tabs */}
      <Tabs defaultValue="configuration" className="w-full">
        <TabsList className="grid w-full grid-cols-4">
          <TabsTrigger value="configuration">Configuration</TabsTrigger>
          <TabsTrigger value="metrics">Metrics</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
          <TabsTrigger value="cost">Cost Analysis</TabsTrigger>
        </TabsList>

        {/* Configuration Tab */}
        <TabsContent value="configuration" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Resource Configuration</CardTitle>
              <CardDescription>Configure CPU, memory, and instance settings</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* CPU Selection */}
              <div className="space-y-2">
                <Label>CPU Units</Label>
                <Select
                  value={formData.cpu.toString()}
                  onValueChange={(value) => {
                    const cpu = parseInt(value);
                    setFormData({
                      ...formData,
                      cpu,
                      memory: getValidMemoryOptions(cpu)[0] || 512,
                    });
                  }}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {[256, 512, 1024, 2048, 4096].map((cpu) => (
                      <SelectItem key={cpu} value={cpu.toString()}>
                        {cpu} CPU ({cpu / 1024} vCPU)
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {/* Memory Selection */}
              <div className="space-y-2">
                <Label>Memory (MB)</Label>
                <Select
                  value={formData.memory.toString()}
                  onValueChange={(value) => setFormData({ ...formData, memory: parseInt(value) })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {getValidMemoryOptions(formData.cpu).map((memory) => (
                      <SelectItem key={memory} value={memory.toString()}>
                        {memory} MB ({(memory / 1024).toFixed(1)} GB)
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {/* Base Instance Count */}
              <div className="space-y-2">
                <Label>Base Instance Count</Label>
                <div className="flex items-center gap-4">
                  <Slider
                    value={[formData.desiredCount]}
                    onValueChange={([value]) => setFormData({ ...formData, desiredCount: value })}
                    min={1}
                    max={20}
                    step={1}
                    className="flex-1"
                  />
                  <span className="w-12 text-right font-medium">{formData.desiredCount}</span>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Autoscaling Settings</CardTitle>
              <CardDescription>Configure automatic scaling behavior</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Enable Autoscaling */}
              <div className="flex items-center justify-between">
                <div className="space-y-0.5">
                  <Label>Enable Autoscaling</Label>
                  <p className="text-sm text-gray-500">
                    Automatically scale based on CPU and memory utilization
                  </p>
                </div>
                <Switch
                  checked={formData.enabled}
                  onCheckedChange={(checked) => setFormData({ ...formData, enabled: checked })}
                />
              </div>

              {formData.enabled && (
                <>
                  {/* Min/Max Capacity */}
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label>Minimum Instances</Label>
                      <Slider
                        value={[formData.minCapacity]}
                        onValueChange={([value]) => setFormData({ ...formData, minCapacity: value })}
                        min={1}
                        max={formData.maxCapacity}
                        step={1}
                      />
                      <div className="text-center text-sm font-medium">{formData.minCapacity}</div>
                    </div>
                    <div className="space-y-2">
                      <Label>Maximum Instances</Label>
                      <Slider
                        value={[formData.maxCapacity]}
                        onValueChange={([value]) => setFormData({ ...formData, maxCapacity: value })}
                        min={formData.minCapacity}
                        max={50}
                        step={1}
                      />
                      <div className="text-center text-sm font-medium">{formData.maxCapacity}</div>
                    </div>
                  </div>

                  {/* Scaling Triggers */}
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <Label>CPU Target Utilization (%)</Label>
                      <div className="flex items-center gap-4">
                        <Slider
                          value={[formData.targetCPU]}
                          onValueChange={([value]) => setFormData({ ...formData, targetCPU: value })}
                          min={10}
                          max={90}
                          step={5}
                          className="flex-1"
                        />
                        <span className="w-12 text-right font-medium">{formData.targetCPU}%</span>
                      </div>
                    </div>

                    <div className="space-y-2">
                      <Label>Memory Target Utilization (%)</Label>
                      <div className="flex items-center gap-4">
                        <Slider
                          value={[formData.targetMemory]}
                          onValueChange={([value]) => setFormData({ ...formData, targetMemory: value })}
                          min={10}
                          max={90}
                          step={5}
                          className="flex-1"
                        />
                        <span className="w-12 text-right font-medium">{formData.targetMemory}%</span>
                      </div>
                    </div>
                  </div>
                </>
              )}
            </CardContent>
          </Card>

          {/* Action Buttons */}
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={fetchData}>
              Reset
            </Button>
            <Button>
              Save Changes
            </Button>
          </div>
        </TabsContent>

        {/* Metrics Tab */}
        <TabsContent value="metrics">
          <Card>
            <CardHeader>
              <CardTitle>Service Metrics</CardTitle>
              <CardDescription>Last 24 hours of performance data</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-[400px]">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" />
                    <YAxis yAxisId="left" />
                    <YAxis yAxisId="right" orientation="right" />
                    <Tooltip />
                    <Legend />
                    <Line
                      yAxisId="left"
                      type="monotone"
                      dataKey="cpu"
                      stroke="#3b82f6"
                      name="CPU %"
                      strokeWidth={2}
                    />
                    <Line
                      yAxisId="left"
                      type="monotone"
                      dataKey="memory"
                      stroke="#10b981"
                      name="Memory %"
                      strokeWidth={2}
                    />
                    <Line
                      yAxisId="right"
                      type="monotone"
                      dataKey="tasks"
                      stroke="#f59e0b"
                      name="Task Count"
                      strokeWidth={2}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* History Tab */}
        <TabsContent value="history">
          <Card>
            <CardHeader>
              <CardTitle>Scaling History</CardTitle>
              <CardDescription>Recent scaling events</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {scalingHistory?.events.map((event, index) => (
                  <div key={index} className="flex items-center gap-4 p-4 border rounded-lg">
                    <div className="flex-shrink-0">
                      {event.activityType === 'ScaleUp' ? (
                        <TrendingUp className="w-8 h-8 text-green-500" />
                      ) : (
                        <TrendingDown className="w-8 h-8 text-red-500" />
                      )}
                    </div>
                    <div className="flex-1">
                      <div className="font-medium">
                        {event.activityType === 'ScaleUp' ? 'Scaled Up' : 'Scaled Down'}
                      </div>
                      <div className="text-sm text-gray-500">
                        From {event.fromCapacity} to {event.toCapacity} instances
                      </div>
                      <div className="text-xs text-gray-400 mt-1">
                        {formatTimestamp(event.timestamp)}
                      </div>
                    </div>
                    <div className="text-right">
                      <Badge variant={event.statusCode === 'Successful' ? 'default' : 'destructive'}>
                        {event.statusCode}
                      </Badge>
                      <div className="text-xs text-gray-500 mt-1">
                        {event.reason}
                      </div>
                    </div>
                  </div>
                )) || (
                  <div className="text-center text-gray-500 py-8">
                    No scaling events in the last 24 hours
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Cost Analysis Tab */}
        <TabsContent value="cost">
          <Card>
            <CardHeader>
              <CardTitle>Cost Estimation</CardTitle>
              <CardDescription>Estimated monthly costs based on configuration</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-6">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <Card>
                    <CardHeader className="pb-2">
                      <CardTitle className="text-sm font-medium text-gray-500">
                        Current Cost
                      </CardTitle>
                    </CardHeader>
                    <CardContent>
                      <div className="flex items-center gap-2">
                        <DollarSign className="w-5 h-5 text-gray-400" />
                        <span className="text-2xl font-bold">
                          ${calculateMonthlyCost(
                            formData.cpu,
                            formData.memory,
                            autoscalingInfo?.currentDesiredCount || 1
                          )}
                        </span>
                      </div>
                      <p className="text-sm text-gray-500 mt-1">
                        {autoscalingInfo?.currentDesiredCount || 1} instance(s)
                      </p>
                    </CardContent>
                  </Card>

                  <Card>
                    <CardHeader className="pb-2">
                      <CardTitle className="text-sm font-medium text-gray-500">
                        Minimum Cost
                      </CardTitle>
                    </CardHeader>
                    <CardContent>
                      <div className="flex items-center gap-2">
                        <DollarSign className="w-5 h-5 text-green-500" />
                        <span className="text-2xl font-bold text-green-600">
                          ${calculateMonthlyCost(
                            formData.cpu,
                            formData.memory,
                            formData.minCapacity
                          )}
                        </span>
                      </div>
                      <p className="text-sm text-gray-500 mt-1">
                        {formData.minCapacity} instance(s)
                      </p>
                    </CardContent>
                  </Card>

                  <Card>
                    <CardHeader className="pb-2">
                      <CardTitle className="text-sm font-medium text-gray-500">
                        Maximum Cost
                      </CardTitle>
                    </CardHeader>
                    <CardContent>
                      <div className="flex items-center gap-2">
                        <DollarSign className="w-5 h-5 text-red-500" />
                        <span className="text-2xl font-bold text-red-600">
                          ${calculateMonthlyCost(
                            formData.cpu,
                            formData.memory,
                            formData.maxCapacity
                          )}
                        </span>
                      </div>
                      <p className="text-sm text-gray-500 mt-1">
                        {formData.maxCapacity} instance(s)
                      </p>
                    </CardContent>
                  </Card>
                </div>

                <Alert>
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>
                    <strong>Note:</strong> These are estimated costs based on AWS Fargate pricing.
                    Actual costs may vary based on region, data transfer, and other factors.
                  </AlertDescription>
                </Alert>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
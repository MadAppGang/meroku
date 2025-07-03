import React, { useEffect, useState } from 'react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Server, Network, Activity, Info, CheckCircle, XCircle, Loader2, AlertCircle, RefreshCw } from 'lucide-react';
import { infrastructureApi, ECSClusterInfo as ClusterInfo, ECSNetworkInfo as NetworkInfo, ECSServicesInfo as ServicesInfo } from '../api/infrastructure';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Switch } from './ui/switch';

interface ECSNodePropertiesProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function ECSNodeProperties({ config, onConfigChange }: ECSNodePropertiesProps) {
  const handleProjectChange = (value: string) => {
    // Validate project name: alphanumeric + dash
    const sanitized = value.toLowerCase().replace(/[^a-z0-9-]/g, '');
    onConfigChange({ project: sanitized });
  };

  const handleRegionChange = (value: string) => {
    // Basic region format validation
    const sanitized = value.toLowerCase().replace(/[^a-z0-9-]/g, '');
    onConfigChange({ region: sanitized });
  };

  const handleStateBucketChange = (value: string) => {
    // S3 bucket naming rules: lowercase, numbers, hyphens, dots
    const sanitized = value.toLowerCase().replace(/[^a-z0-9.-]/g, '');
    onConfigChange({ state_bucket: sanitized });
  };

  const handleStateFileChange = (value: string) => {
    // State file name validation
    const sanitized = value.replace(/[^a-zA-Z0-9._-]/g, '');
    onConfigChange({ state_file: sanitized });
  };

  return (
    <div className="space-y-6">
      {/* Project Configuration */}
      <Card>
        <CardHeader>
          <CardTitle>Project Configuration</CardTitle>
          <CardDescription>
            Core infrastructure settings for this environment
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 gap-4">
            <div>
              <Label htmlFor="project-name">Project Name</Label>
              <Input
                id="project-name"
                value={config.project}
                onChange={(e) => handleProjectChange(e.target.value)}
                className="mt-1 bg-gray-800 border-gray-600 text-white font-mono"
                placeholder="my-app"
              />
              <p className="text-xs text-gray-500 mt-1">Alphanumeric + dash (e.g., "my-app")</p>
            </div>
            
            <div>
              <Label htmlFor="environment">Environment</Label>
              <Input
                id="environment"
                value={config.env}
                className="mt-1 bg-gray-800 border-gray-600 text-white font-mono"
                disabled
                readOnly
              />
              <p className="text-xs text-gray-500 mt-1">Environment name cannot be changed</p>
            </div>
            
            <div className="flex items-center justify-between">
              <div className="flex-1">
                <Label htmlFor="is-prod">Production Environment</Label>
                <p className="text-xs text-gray-500 mt-1">Affects domain behavior and security settings</p>
              </div>
              <Switch
                id="is-prod"
                checked={config.is_prod || false}
                onCheckedChange={(checked) => onConfigChange({ is_prod: checked })}
                className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
              />
            </div>
            
            <div>
              <Label htmlFor="region">AWS Region</Label>
              <Input
                id="region"
                value={config.region}
                onChange={(e) => handleRegionChange(e.target.value)}
                className="mt-1 bg-gray-800 border-gray-600 text-white font-mono"
                placeholder="us-east-1"
              />
              <p className="text-xs text-gray-500 mt-1">AWS region for all resources (e.g., us-east-1, eu-central-1)</p>
            </div>
            
            <div>
              <Label htmlFor="state-bucket">Terraform State Bucket</Label>
              <Input
                id="state-bucket"
                value={config.state_bucket}
                onChange={(e) => handleStateBucketChange(e.target.value)}
                className="mt-1 bg-gray-800 border-gray-600 text-white font-mono"
                placeholder="my-terraform-state-bucket"
              />
              <p className="text-xs text-gray-500 mt-1">S3 bucket for Terraform state storage</p>
            </div>
            
            <div>
              <Label htmlFor="state-file">State File</Label>
              <Input
                id="state-file"
                value={config.state_file}
                onChange={(e) => handleStateFileChange(e.target.value)}
                className="mt-1 bg-gray-800 border-gray-600 text-white font-mono"
                placeholder="state.tfstate"
              />
              <p className="text-xs text-gray-500 mt-1">Terraform state file name</p>
            </div>
          </div>

          {/* Warning for production changes */}
          {config.is_prod && (
            <div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
              <div className="flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-yellow-400 mt-0.5" />
                <div className="flex-1">
                  <h4 className="text-sm font-medium text-yellow-400 mb-1">Production Environment Active</h4>
                  <p className="text-xs text-gray-300">
                    This environment is marked as production. This affects domain prefixing and other security settings.
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* Infrastructure Impact Warning */}
          <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-blue-400 mt-0.5" />
              <div className="flex-1">
                <h4 className="text-sm font-medium text-blue-400 mb-1">Infrastructure Impact</h4>
                <p className="text-xs text-gray-300 mb-2">
                  Changing these values requires infrastructure updates:
                </p>
                <ul className="text-xs text-gray-400 space-y-1">
                  <li>• <strong>Project/Region:</strong> Requires destroying and recreating all resources</li>
                  <li>• <strong>State Bucket/File:</strong> Requires Terraform state migration</li>
                  <li>• <strong>Production Flag:</strong> May affect domain configuration and security policies</li>
                </ul>
                <p className="text-xs text-gray-300 mt-2">
                  Always run <code className="text-blue-300">make infra-plan</code> before applying changes.
                </p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* ECS Cluster Configuration */}
      <Card>
        <CardHeader>
          <CardTitle>Amazon ECS Configuration</CardTitle>
          <CardDescription>
            Elastic Container Service cluster settings
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Overview */}
          <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-blue-400 mt-0.5" />
              <div className="flex-1">
                <h4 className="text-sm font-medium text-blue-400 mb-2">Cluster Overview</h4>
                <p className="text-xs text-gray-300">
                  This ECS cluster runs all your containerized services using AWS Fargate. 
                  It provides serverless compute for containers with automatic scaling and high availability.
                </p>
              </div>
            </div>
          </div>

          {/* Key Features */}
          <div className="space-y-3">
            <h4 className="text-sm font-medium text-gray-300">Key Features</h4>
            <div className="grid grid-cols-1 gap-2">
              <div className="flex items-center gap-2">
                <CheckCircle className="w-4 h-4 text-green-400" />
                <span className="text-xs text-gray-300">Fargate launch type (serverless containers)</span>
              </div>
              <div className="flex items-center gap-2">
                <CheckCircle className="w-4 h-4 text-green-400" />
                <span className="text-xs text-gray-300">Service Connect for service discovery</span>
              </div>
              <div className="flex items-center gap-2">
                <CheckCircle className="w-4 h-4 text-green-400" />
                <span className="text-xs text-gray-300">Multi-AZ deployment for high availability</span>
              </div>
              <div className="flex items-center gap-2">
                <CheckCircle className="w-4 h-4 text-green-400" />
                <span className="text-xs text-gray-300">Integrated with CloudWatch Container Insights</span>
              </div>
            </div>
          </div>

          {/* Service Discovery */}
          <div className="bg-gray-800 rounded-lg p-4">
            <h4 className="text-sm font-medium text-gray-300 mb-2">Service Discovery</h4>
            <p className="text-xs text-gray-400">
              Services use AWS Cloud Map for service discovery with the namespace <code className="text-blue-300">local</code>. 
              This allows services to communicate using DNS names like <code className="text-blue-300">backend.local</code>.
            </p>
          </div>

          {/* Auto Scaling */}
          <div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
            <h4 className="text-sm font-medium text-yellow-400 mb-2">Auto Scaling</h4>
            <p className="text-xs text-gray-300">
              ECS services are configured with auto-scaling policies based on CPU and memory utilization. 
              Services automatically scale in and out based on demand.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export function ECSClusterInfo({ config }: { config: YamlInfrastructureConfig }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [clusterInfo, setClusterInfo] = useState<ClusterInfo | null>(null);

  useEffect(() => {
    const fetchClusterInfo = async () => {
      try {
        setLoading(true);
        setError(null);
        const data = await infrastructureApi.getECSClusterInfo(config.env);
        setClusterInfo(data);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch cluster info');
      } finally {
        setLoading(false);
      }
    };

    fetchClusterInfo();
  }, [config.env]);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-8">
        <Loader2 className="w-6 h-6 text-blue-400 animate-spin" />
        <span className="ml-2 text-sm text-gray-400">Loading cluster information...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-900/20 border border-red-700 rounded-lg p-4">
        <div className="flex items-start gap-2">
          <AlertCircle className="w-4 h-4 text-red-400 mt-0.5" />
          <div className="flex-1">
            <h4 className="text-sm font-medium text-red-400 mb-1">Error Loading Cluster Info</h4>
            <p className="text-xs text-gray-300">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  if (!clusterInfo) return null;

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <Server className="w-5 h-5 text-blue-400" />
              Cluster Information
            </CardTitle>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setLoading(true);
                setError(null);
                infrastructureApi.getECSClusterInfo(config.env)
                  .then(setClusterInfo)
                  .catch(err => setError(err instanceof Error ? err.message : 'Failed to fetch cluster info'))
                  .finally(() => setLoading(false));
              }}
              disabled={loading}
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 gap-4">
            <div>
              <label className="text-xs text-gray-400">Cluster Name</label>
              <p className="text-sm font-mono text-gray-300">{clusterInfo.clusterName}</p>
            </div>
            
            <div>
              <label className="text-xs text-gray-400">Status</label>
              <p className="text-sm text-gray-300">
                <span className={`inline-flex items-center gap-1 ${clusterInfo.status === 'ACTIVE' ? 'text-green-400' : 'text-yellow-400'}`}>
                  {clusterInfo.status === 'ACTIVE' ? <CheckCircle className="w-3 h-3" /> : <AlertCircle className="w-3 h-3" />}
                  {clusterInfo.status}
                </span>
              </p>
            </div>
            
            <div>
              <label className="text-xs text-gray-400">Region</label>
              <p className="text-sm text-gray-300">{config.region}</p>
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div>
                <label className="text-xs text-gray-400">Running Tasks</label>
                <p className="text-2xl font-medium text-gray-300">{clusterInfo.runningTasks}</p>
              </div>
              <div>
                <label className="text-xs text-gray-400">Active Services</label>
                <p className="text-2xl font-medium text-gray-300">{clusterInfo.activeServices}</p>
              </div>
              <div>
                <label className="text-xs text-gray-400">Registered Tasks</label>
                <p className="text-2xl font-medium text-gray-300">{clusterInfo.registeredTasks}</p>
              </div>
            </div>

            <div>
              <label className="text-xs text-gray-400">Container Insights</label>
              <p className="text-sm text-gray-300">
                <span className={`inline-flex items-center gap-1 ${clusterInfo.containerInsights === 'enabled' ? 'text-green-400' : 'text-gray-400'}`}>
                  {clusterInfo.containerInsights === 'enabled' ? <CheckCircle className="w-3 h-3" /> : <XCircle className="w-3 h-3" />}
                  {clusterInfo.containerInsights === 'enabled' ? 'Enabled' : 'Disabled'}
                </span>
              </p>
            </div>
            
            <div>
              <label className="text-xs text-gray-400">Compute Configuration</label>
              <div className="mt-1 bg-gray-800 rounded p-3">
                <ul className="text-xs text-gray-400 space-y-1">
                  <li>• Serverless compute with AWS Fargate</li>
                  <li>• No EC2 instances to manage</li>
                  <li>• Pay only for resources used by containers</li>
                  <li>• Automatic OS and runtime patching</li>
                </ul>
              </div>
            </div>
          </div>

          {/* Capacity Providers */}
          <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <h4 className="text-sm font-medium text-blue-400 mb-2">Capacity Providers</h4>
            <div className="space-y-2">
              {clusterInfo.capacityProviders.map((provider) => (
                <div key={provider} className="flex items-center justify-between text-xs">
                  <span className="text-gray-300">{provider}</span>
                  <span className="text-green-400">Active</span>
                </div>
              ))}
            </div>
          </div>

          {/* Cluster ARN */}
          <div className="bg-gray-800 rounded-lg p-3">
            <label className="text-xs text-gray-400">Cluster ARN</label>
            <p className="text-xs font-mono text-gray-300 mt-1 break-all">{clusterInfo.clusterArn}</p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export function ECSNetworkInfo({ config }: { config: YamlInfrastructureConfig }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [networkInfo, setNetworkInfo] = useState<NetworkInfo | null>(null);

  useEffect(() => {
    const fetchNetworkInfo = async () => {
      try {
        setLoading(true);
        setError(null);
        const data = await infrastructureApi.getECSNetworkInfo(config.env);
        setNetworkInfo(data);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch network info');
      } finally {
        setLoading(false);
      }
    };

    fetchNetworkInfo();
  }, [config.env]);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-8">
        <Loader2 className="w-6 h-6 text-purple-400 animate-spin" />
        <span className="ml-2 text-sm text-gray-400">Loading network information...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-900/20 border border-red-700 rounded-lg p-4">
        <div className="flex items-start gap-2">
          <AlertCircle className="w-4 h-4 text-red-400 mt-0.5" />
          <div className="flex-1">
            <h4 className="text-sm font-medium text-red-400 mb-1">Error Loading Network Info</h4>
            <p className="text-xs text-gray-300">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  if (!networkInfo) return null;

  // Group subnets by type
  const privateSubnets = networkInfo.subnets.filter(s => s.type === 'private');
  const publicSubnets = networkInfo.subnets.filter(s => s.type === 'public');

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <Network className="w-5 h-5 text-purple-400" />
              Network Details
            </CardTitle>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setLoading(true);
                setError(null);
                infrastructureApi.getECSNetworkInfo(config.env)
                  .then(setNetworkInfo)
                  .catch(err => setError(err instanceof Error ? err.message : 'Failed to fetch network info'))
                  .finally(() => setLoading(false));
              }}
              disabled={loading}
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 gap-4">
            <div>
              <label className="text-xs text-gray-400">VPC</label>
              <p className="text-sm text-gray-300">VPC ID: {networkInfo.vpc.vpcId}</p>
              <p className="text-xs text-gray-500 mt-1">CIDR: {networkInfo.vpc.cidrBlock}</p>
              <p className="text-xs text-gray-500">State: <span className="text-green-400">{networkInfo.vpc.state}</span></p>
            </div>
            
            <div>
              <label className="text-xs text-gray-400">Availability Zones</label>
              <div className="flex gap-2 mt-1">
                {networkInfo.availabilityZones.map((az) => (
                  <span key={az} className="px-2 py-1 bg-purple-900/30 border border-purple-700 rounded text-xs text-purple-300">
                    {az}
                  </span>
                ))}
              </div>
            </div>
            
            <div>
              <label className="text-xs text-gray-400">Subnet Configuration</label>
              <div className="mt-1 space-y-2">
                {privateSubnets.length > 0 && (
                  <div className="bg-gray-800 rounded p-3">
                    <h5 className="text-xs font-medium text-gray-300 mb-2">Private Subnets ({privateSubnets.length})</h5>
                    <ul className="text-xs text-gray-500 space-y-1">
                      {privateSubnets.map((subnet) => (
                        <li key={subnet.subnetId}>
                          • {subnet.cidrBlock} ({subnet.availabilityZone}) - {subnet.availableIpCount} IPs available
                        </li>
                      ))}
                    </ul>
                    <p className="text-xs text-gray-400 mt-2">Used for ECS tasks and RDS instances</p>
                  </div>
                )}
                {publicSubnets.length > 0 && (
                  <div className="bg-gray-800 rounded p-3">
                    <h5 className="text-xs font-medium text-gray-300 mb-2">Public Subnets ({publicSubnets.length})</h5>
                    <ul className="text-xs text-gray-500 space-y-1">
                      {publicSubnets.map((subnet) => (
                        <li key={subnet.subnetId}>
                          • {subnet.cidrBlock} ({subnet.availabilityZone}) - {subnet.availableIpCount} IPs available
                        </li>
                      ))}
                    </ul>
                    <p className="text-xs text-gray-400 mt-2">Used for NAT gateways and load balancers</p>
                  </div>
                )}
              </div>
            </div>
            
            {networkInfo.serviceDiscovery && (
              <div>
                <label className="text-xs text-gray-400">Service Discovery Namespace</label>
                <p className="text-sm font-mono text-gray-300">{networkInfo.serviceDiscovery.namespaceName}</p>
                <p className="text-xs text-gray-500 mt-1">
                  {networkInfo.serviceDiscovery.serviceCount} service{networkInfo.serviceDiscovery.serviceCount !== 1 ? 's' : ''} registered
                </p>
                <p className="text-xs text-gray-500">
                  Services can discover each other using DNS names like <code>service-name.{networkInfo.serviceDiscovery.namespaceName}</code>
                </p>
              </div>
            )}
          </div>

          {/* Subnet Details */}
          <div className="bg-gray-800 rounded-lg p-4">
            <h4 className="text-sm font-medium text-gray-300 mb-3">Subnet Details</h4>
            <div className="space-y-2">
              {networkInfo.subnets.map((subnet) => (
                <div key={subnet.subnetId} className="text-xs">
                  <p className="font-mono text-gray-400">{subnet.subnetId}</p>
                  <p className="text-gray-500 ml-2">
                    {subnet.type === 'private' ? '🔒' : '🌐'} {subnet.cidrBlock} • {subnet.availabilityZone} • {subnet.availableIpCount} IPs
                  </p>
                </div>
              ))}
            </div>
          </div>

          {/* Security Groups */}
          <div className="bg-purple-900/20 border border-purple-700 rounded-lg p-4">
            <h4 className="text-sm font-medium text-purple-400 mb-2">Security Configuration</h4>
            <ul className="text-xs text-gray-300 space-y-1">
              <li>• Dedicated security groups per service</li>
              <li>• Least privilege access between services</li>
              <li>• No direct internet access for tasks (NAT Gateway)</li>
              <li>• ALB handles public traffic termination</li>
            </ul>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export function ECSServicesInfo({ config }: { config: YamlInfrastructureConfig }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [servicesInfo, setServicesInfo] = useState<ServicesInfo | null>(null);

  useEffect(() => {
    const fetchServicesInfo = async () => {
      try {
        setLoading(true);
        setError(null);
        const data = await infrastructureApi.getECSServicesInfo(config.env);
        setServicesInfo(data);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch services info');
      } finally {
        setLoading(false);
      }
    };

    fetchServicesInfo();
  }, [config.env]);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-8">
        <Loader2 className="w-6 h-6 text-green-400 animate-spin" />
        <span className="ml-2 text-sm text-gray-400">Loading services information...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-900/20 border border-red-700 rounded-lg p-4">
        <div className="flex items-start gap-2">
          <AlertCircle className="w-4 h-4 text-red-400 mt-0.5" />
          <div className="flex-1">
            <h4 className="text-sm font-medium text-red-400 mb-1">Error Loading Services Info</h4>
            <p className="text-xs text-gray-300">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  if (!servicesInfo) return null;

  const runningServicesCount = servicesInfo.services.length;
  const scheduledTasksCount = servicesInfo.scheduledTasks.length;
  const eventTasksCount = servicesInfo.eventTasks.length;

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <Activity className="w-5 h-5 text-green-400" />
              Services Running
            </CardTitle>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setLoading(true);
                setError(null);
                infrastructureApi.getECSServicesInfo(config.env)
                  .then(setServicesInfo)
                  .catch(err => setError(err instanceof Error ? err.message : 'Failed to fetch services info'))
                  .finally(() => setLoading(false));
              }}
              disabled={loading}
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Service Count */}
          <div className="grid grid-cols-2 gap-4">
            <div className="bg-gray-800 rounded-lg p-4">
              <p className="text-xs text-gray-400 mb-2">Active Services</p>
              <p className="text-2xl font-medium text-gray-300 mb-3">{runningServicesCount}</p>
              <div className="space-y-1 max-h-32 overflow-y-auto">
                {servicesInfo.services.map((service) => (
                  <p key={service.serviceName} className="text-xs text-gray-500 truncate" title={service.serviceName}>
                    • {service.serviceName}
                  </p>
                ))}
              </div>
            </div>
            
            <div className="bg-gray-800 rounded-lg p-4">
              <p className="text-xs text-gray-400 mb-2">Total Tasks</p>
              <p className="text-2xl font-medium text-gray-300 mb-3">{servicesInfo.totalTasks}</p>
              <div className="space-y-1">
                <p className="text-xs text-gray-500">• Service Tasks: {runningServicesCount}</p>
                {scheduledTasksCount > 0 && (
                  <p className="text-xs text-gray-500">• Scheduled Tasks: {scheduledTasksCount}</p>
                )}
                {eventTasksCount > 0 && (
                  <p className="text-xs text-gray-500">• Event Tasks: {eventTasksCount}</p>
                )}
              </div>
            </div>
          </div>

          {/* Service List */}
          {servicesInfo.services.length > 0 && (
            <div>
              <h4 className="text-sm font-medium text-gray-300 mb-3">Deployed Services</h4>
              <div className="space-y-2">
                {servicesInfo.services.map((service) => (
                  <div key={service.serviceName} className="p-3 bg-gray-800 rounded">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <div className={`w-2 h-2 rounded-full flex-shrink-0 ${
                          service.status === 'ACTIVE' ? 'bg-green-400' : 
                          service.status === 'DRAINING' ? 'bg-yellow-400' : 
                          'bg-red-400'
                        }`}></div>
                        <div className="flex-1 min-w-0">
                          <p className="text-sm text-gray-300 truncate" title={service.serviceName}>{service.serviceName}</p>
                          <p className="text-xs text-gray-500">{service.launchType} • {service.taskDefinition.split('/').pop()}</p>
                        </div>
                      </div>
                      <span className={`text-xs ${
                        service.status === 'ACTIVE' ? 'text-green-400' : 
                        service.status === 'DRAINING' ? 'text-yellow-400' : 
                        'text-red-400'
                      }`}>{service.status}</span>
                    </div>
                    <div className="flex gap-4 mt-2 text-xs text-gray-400">
                      <span>Desired: {service.desiredCount}</span>
                      <span>Running: {service.runningCount}</span>
                      <span>Pending: {service.pendingCount}</span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Scheduled Tasks */}
          {servicesInfo.scheduledTasks.length > 0 && (
            <div>
              <h4 className="text-sm font-medium text-gray-300 mb-3">Scheduled Tasks</h4>
              <div className="space-y-2">
                {servicesInfo.scheduledTasks.map((task) => (
                  <div key={task.taskName} className="p-3 bg-gray-800 rounded">
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex-1 min-w-0">
                        <p className="text-sm text-gray-300 truncate" title={task.taskName}>{task.taskName}</p>
                        <p className="text-xs text-gray-500">{task.schedule}</p>
                      </div>
                      <span className={`text-xs flex-shrink-0 ${task.enabled ? 'text-green-400' : 'text-gray-400'}`}>
                        {task.enabled ? 'Enabled' : 'Disabled'}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Event Tasks */}
          {servicesInfo.eventTasks.length > 0 && (
            <div>
              <h4 className="text-sm font-medium text-gray-300 mb-3">Event-Driven Tasks</h4>
              <div className="space-y-2">
                {servicesInfo.eventTasks.map((task) => (
                  <div key={task.taskName} className="p-3 bg-gray-800 rounded">
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex-1 min-w-0">
                        <p className="text-sm text-gray-300 truncate" title={task.taskName}>{task.taskName}</p>
                        <p className="text-xs text-gray-500">Event pattern configured</p>
                      </div>
                      <span className={`text-xs flex-shrink-0 ${task.enabled ? 'text-green-400' : 'text-gray-400'}`}>
                        {task.enabled ? 'Enabled' : 'Disabled'}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Resource Utilization */}
          <div className="bg-green-900/20 border border-green-700 rounded-lg p-4">
            <h4 className="text-sm font-medium text-green-400 mb-2">Resource Allocation</h4>
            <p className="text-xs text-gray-300">
              Each service is allocated CPU and memory based on workload requirements. 
              Fargate automatically provisions the right compute resources.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
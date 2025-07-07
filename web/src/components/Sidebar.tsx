import React, { useState, useContext, useRef, useEffect } from 'react';
import { X, Settings, FileText, BarChart, Zap, Link, Code, Database, Upload, Globe, BookOpen, Key, HardDrive, Shield, Server, Network, Activity, ChevronLeft, ChevronRight, Bell, Microscope, Gauge, Terminal, Cloud, Info, Send } from 'lucide-react';
import { ComponentNode } from '../types';
import { Tabs } from './ui/tabs';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Switch } from './ui/switch';
import { ECSNodeProperties, ECSClusterInfo, ECSNetworkInfo, ECSServicesInfo, ECSNotifications } from './ECSNodeProperties';
import { BackendServiceProperties } from './BackendServiceProperties';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { type AccountInfo } from '../api/infrastructure';
import { NodeConfigProperties } from './NodeConfigProperties';
import { GitHubNodeProperties } from './GitHubNodeProperties';
import { ECRNodeProperties } from './ECRNodeProperties';
import { ECRRepositoryList } from './ECRRepositoryList';
import { ECRPushInstructions } from './ECRPushInstructions';
import { Route53NodeProperties } from './Route53NodeProperties';
import { Route53DNSRecords } from './Route53DNSRecords';
import { AuroraNodeProperties } from './AuroraNodeProperties';
import { ParameterStoreNodeProperties } from './ParameterStoreNodeProperties';
import { ParameterStoreDescription } from './ParameterStoreDescription';
import { BackendEnvironmentVariables } from './BackendEnvironmentVariables';
import { BackendParameterStore } from './BackendParameterStore';
import { BackendS3Buckets } from './BackendS3Buckets';
import { BackendIAMPermissions } from './BackendIAMPermissions';
import { BackendXRayConfiguration } from './BackendXRayConfiguration';
import { BackendScalingConfiguration } from './BackendScalingConfiguration';
import { BackendSSHAccess } from './BackendSSHAccess';
import { BackendCloudWatch } from './BackendCloudWatch';
import { BackendAlerts } from './BackendAlerts';
import { ServiceLogs } from './ServiceLogs';
import { ScheduledTaskProperties } from './ScheduledTaskProperties';
import { ScheduledTaskParameterStore } from './ScheduledTaskParameterStore';
import { ScheduledTaskCloudWatch } from './ScheduledTaskCloudWatch';
import { ScheduledTaskIAMPermissions } from './ScheduledTaskIAMPermissions';
import { EventTaskProperties } from './EventTaskProperties';
import { EventTaskTestEvent } from './EventTaskTestEvent';
import { SESNodeProperties } from './SESNodeProperties';
import { SESStatus } from './SESStatus';
import { SESSendTestEmail } from './SESSendTestEmail';
import { S3NodeProperties } from './S3NodeProperties';
import { PostgresNodeProperties } from './PostgresNodeProperties';

interface SidebarProps {
  selectedNode: ComponentNode | null;
  isOpen: boolean;
  onClose: () => void;
  config?: YamlInfrastructureConfig;
  onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
  accountInfo?: AccountInfo;
}

export function Sidebar({ selectedNode, isOpen, onClose, config, onConfigChange, accountInfo }: SidebarProps) {
  const [activeTab, setActiveTab] = useState('settings');
  const [showLeftScroll, setShowLeftScroll] = useState(false);
  const [showRightScroll, setShowRightScroll] = useState(false);
  const tabsContainerRef = useRef<HTMLDivElement>(null);

  const checkScrollButtons = () => {
    if (tabsContainerRef.current) {
      const { scrollLeft, scrollWidth, clientWidth } = tabsContainerRef.current;
      setShowLeftScroll(scrollLeft > 0);
      setShowRightScroll(scrollLeft < scrollWidth - clientWidth - 5);
    }
  };

  useEffect(() => {
    checkScrollButtons();
    const container = tabsContainerRef.current;
    if (container) {
      container.addEventListener('scroll', checkScrollButtons);
      window.addEventListener('resize', checkScrollButtons);
      return () => {
        container.removeEventListener('scroll', checkScrollButtons);
        window.removeEventListener('resize', checkScrollButtons);
      };
    }
  }, [selectedNode]);

  const scrollTabs = (direction: 'left' | 'right') => {
    if (tabsContainerRef.current) {
      const scrollAmount = 150;
      tabsContainerRef.current.scrollBy({
        left: direction === 'left' ? -scrollAmount : scrollAmount,
        behavior: 'smooth'
      });
    }
  };

  if (!isOpen || !selectedNode) return null;

  return (
    <div className="fixed right-0 top-0 h-full w-[768px] bg-gray-900 border-l border-gray-700 shadow-xl z-50 flex flex-col">
      <div className="flex items-start justify-between p-4 border-b border-gray-700 flex-shrink-0">
        <div className="flex-1 pr-2">
          <h2 className="text-lg font-medium text-white">{selectedNode.name}</h2>
          {config && (
            <div className="grid grid-cols-3 gap-2 mt-2">
              <div>
                <span className="text-xs text-gray-400 block">Project</span>
                <span className="text-xs text-gray-300 font-mono">{config.project}</span>
              </div>
              <div>
                <span className="text-xs text-gray-400 block">Region</span>
                <span className="text-xs text-gray-300 font-mono">{config.region}</span>
              </div>
              <div>
                <span className="text-xs text-gray-400 block">Environment</span>
                <span className="text-xs text-gray-300 font-mono">{config.env}</span>
              </div>
            </div>
          )}
        </div>
        <Button
          variant="ghost"
          size="icon"
          onClick={onClose}
          className="text-gray-400 hover:text-white flex-shrink-0"
        >
          <X className="w-4 h-4" />
        </Button>
      </div>

      <div className="relative flex-shrink-0 border-b border-gray-700">
        {/* Left scroll button */}
        {showLeftScroll && (
          <button
            onClick={() => scrollTabs('left')}
            className="absolute left-0 top-0 bottom-0 z-10 px-2 bg-gradient-to-r from-gray-900 via-gray-900 to-transparent flex items-center justify-center"
          >
            <ChevronLeft className="w-4 h-4 text-gray-400" />
          </button>
        )}
        
        {/* Right scroll button */}
        {showRightScroll && (
          <button
            onClick={() => scrollTabs('right')}
            className="absolute right-0 top-0 bottom-0 z-10 px-2 bg-gradient-to-l from-gray-900 via-gray-900 to-transparent flex items-center justify-center"
          >
            <ChevronRight className="w-4 h-4 text-gray-400" />
          </button>
        )}

        <div 
          ref={tabsContainerRef}
          className="overflow-x-auto scrollbar-hide"
          style={{ scrollbarWidth: 'none', msOverflowStyle: 'none' }}
        >
          <div className="flex min-w-max">
            {(selectedNode.type === 'github' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'example', label: 'Example', icon: Code },
            ] : selectedNode.type === 'ecr' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'repos', label: 'Repos', icon: Database },
              { id: 'push', label: 'Push', icon: Upload },
            ] : selectedNode.type === 'route53' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'dns', label: 'DNS', icon: Globe },
            ] : selectedNode.type === 'aurora' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
            ] : selectedNode.type === 'secrets-manager' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'description', label: 'Description', icon: BookOpen },
            ] : selectedNode.type === 'ses' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'status', label: 'Status', icon: Activity },
              { id: 'send', label: 'Send Email', icon: Send },
            ] : selectedNode.type === 's3' ? [
              { id: 'settings', label: 'Buckets', icon: HardDrive },
            ] : selectedNode.type === 'postgres' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
            ] : selectedNode.type === 'ecs' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'notifications', label: 'Notifications', icon: Bell },
              { id: 'cluster', label: 'Cluster', icon: Server },
              { id: 'network', label: 'Network', icon: Network },
              { id: 'services', label: 'Services', icon: Activity },
            ] : selectedNode.type === 'backend' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'scaling', label: 'Scaling', icon: Gauge },
              { id: 'xray', label: 'X-Ray', icon: Microscope },
              { id: 'ssh', label: 'SSH', icon: Terminal },
              { id: 'env', label: 'Env Vars', icon: Zap },
              { id: 'params', label: 'Parameters', icon: Key },
              { id: 's3', label: 'S3 Buckets', icon: HardDrive },
              { id: 'iam', label: 'IAM', icon: Shield },
              { id: 'logs', label: 'Logs', icon: FileText },
              { id: 'cloudwatch', label: 'CloudWatch', icon: Cloud },
              { id: 'alerts', label: 'Alerts', icon: Bell },
            ] : selectedNode.type === 'scheduled-task' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'env', label: 'Env Vars', icon: Zap },
              { id: 'params', label: 'Parameters', icon: Key },
              { id: 'iam', label: 'IAM', icon: Shield },
              { id: 'cloudwatch', label: 'CloudWatch', icon: Cloud },
              { id: 'logs', label: 'Logs', icon: FileText },
            ] : selectedNode.type === 'event-task' ? [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'test', label: 'Test Event', icon: Send },
              { id: 'env', label: 'Env Vars', icon: Zap },
              { id: 'params', label: 'Parameters', icon: Key },
              { id: 'iam', label: 'IAM', icon: Shield },
              { id: 'cloudwatch', label: 'CloudWatch', icon: Cloud },
              { id: 'logs', label: 'Logs', icon: FileText },
            ] : [
              { id: 'settings', label: 'Settings', icon: Settings },
              { id: 'logs', label: 'Logs', icon: FileText },
              { id: 'metrics', label: 'Metrics', icon: BarChart },
              { id: 'env', label: 'Environment', icon: Zap },
              { id: 'connections', label: 'Connections', icon: Link },
            ]).map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center justify-center gap-2 px-4 py-2 text-sm font-medium transition-colors whitespace-nowrap ${
                  activeTab === tab.id
                    ? 'text-blue-400 border-b-2 border-blue-400'
                    : 'text-gray-400 hover:text-white'
                }`}
              >
                <tab.icon className="w-4 h-4 flex-shrink-0" />
                <span>{tab.label}</span>
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4 min-h-0">
        {activeTab === 'settings' && (
          selectedNode.type === 'ecs' && config && onConfigChange ? (
            <ECSNodeProperties 
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === 'backend' && config && onConfigChange ? (
            <BackendServiceProperties 
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
            />
          ) : selectedNode.type === 'github' && config && onConfigChange ? (
            <GitHubNodeProperties 
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === 'ecr' && config && onConfigChange ? (
            <ECRNodeProperties 
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
            />
          ) : selectedNode.type === 'route53' && config && onConfigChange ? (
            <Route53NodeProperties 
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === 'aurora' ? (
            <AuroraNodeProperties />
          ) : selectedNode.type === 'secrets-manager' && config ? (
            <ParameterStoreNodeProperties config={config} />
          ) : selectedNode.type === 'ses' && config && onConfigChange ? (
            <SESNodeProperties 
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === 's3' && config && onConfigChange ? (
            <S3NodeProperties config={config} onConfigChange={onConfigChange} />
          ) : selectedNode.type === 'postgres' && config && onConfigChange ? (
            <PostgresNodeProperties config={config} onConfigChange={onConfigChange} accountInfo={accountInfo} />
          ) : selectedNode.type === 'scheduled-task' && config && onConfigChange ? (
            <ScheduledTaskProperties 
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
              node={selectedNode}
            />
          ) : selectedNode.type === 'event-task' && config && onConfigChange ? (
            <EventTaskProperties 
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
              node={selectedNode}
            />
          ) : (
            <div className="space-y-6">
              <div>
                <Label htmlFor="service-name">Service Name</Label>
                <Input
                  id="service-name"
                  value={selectedNode.name}
                  className="mt-1 bg-gray-800 border-gray-600 text-white"
                  readOnly
                />
              </div>
              
              <div>
                <Label htmlFor="service-url">Service URL</Label>
                <Input
                  id="service-url"
                  value={selectedNode.url || 'Not available'}
                  className="mt-1 bg-gray-800 border-gray-600 text-white"
                  readOnly
                />
              </div>

              <div>
                <Label>Status</Label>
                <div className="mt-1 px-3 py-2 bg-gray-800 border border-gray-600 rounded-md">
                  <span className={`capitalize ${
                    selectedNode.status === 'running' ? 'text-green-400' :
                    selectedNode.status === 'deploying' ? 'text-yellow-400' :
                    selectedNode.status === 'error' ? 'text-red-400' : 'text-gray-400'
                  }`}>
                    {selectedNode.status}
                  </span>
                </div>
              </div>

              <div className="flex items-center justify-between">
                <Label htmlFor="auto-deploy">Auto Deploy</Label>
                <Switch id="auto-deploy" defaultChecked />
              </div>

              <div className="flex items-center justify-between">
                <Label htmlFor="health-check">Health Check</Label>
                <Switch id="health-check" defaultChecked />
              </div>

              {selectedNode.replicas && (
                <div>
                  <Label htmlFor="replicas">Replicas</Label>
                  <Input
                    id="replicas"
                    type="number"
                    value={selectedNode.replicas}
                    className="mt-1 bg-gray-800 border-gray-600 text-white"
                    readOnly
                  />
                </div>
              )}
              
              {/* Show configuration properties if available */}
              {selectedNode.configProperties && (
                <div className="mt-6">
                  <NodeConfigProperties node={selectedNode} />
                </div>
              )}
            </div>
          )
        )}

        {activeTab === 'logs' && config && (
          selectedNode.type === 'backend' ? (
            <ServiceLogs environment={config.env} serviceName="backend" />
          ) : selectedNode.type === 'service' ? (
            <ServiceLogs environment={config.env} serviceName={selectedNode.name} />
          ) : null
        )}

        {activeTab === 'metrics' && (
          <div className="space-y-6">
            <div>
              <h3 className="font-medium text-white mb-4">Performance Metrics</h3>
              <div className="grid grid-cols-2 gap-4">
                <div className="bg-gray-800 p-3 rounded-lg">
                  <div className="text-sm text-gray-400">CPU Usage</div>
                  <div className="text-2xl font-medium text-white">23%</div>
                </div>
                <div className="bg-gray-800 p-3 rounded-lg">
                  <div className="text-sm text-gray-400">Memory</div>
                  <div className="text-2xl font-medium text-white">1.2GB</div>
                </div>
                <div className="bg-gray-800 p-3 rounded-lg">
                  <div className="text-sm text-gray-400">Requests/min</div>
                  <div className="text-2xl font-medium text-white">1.2K</div>
                </div>
                <div className="bg-gray-800 p-3 rounded-lg">
                  <div className="text-sm text-gray-400">Uptime</div>
                  <div className="text-2xl font-medium text-white">99.9%</div>
                </div>
              </div>
            </div>
          </div>
        )}


        {activeTab === 'connections' && (
          <div className="space-y-4">
            <h3 className="font-medium text-white">Connected Services</h3>
            <div className="space-y-2">
              {['database', 'cache', 'api-gateway'].map((service) => (
                <div key={service} className="bg-gray-800 p-3 rounded-lg flex items-center gap-3">
                  <div className="w-2 h-2 bg-green-400 rounded-full"></div>
                  <span className="text-white">{service}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {activeTab === 'example' && selectedNode.type === 'github' && (
          <div className="space-y-4">
            <h3 className="font-medium text-white mb-4">GitHub Actions Workflow Example</h3>
            <div className="bg-gray-800 rounded-lg p-4">
              <div className="flex items-center justify-between mb-3">
                <span className="text-sm text-gray-400 font-mono">.github/workflows/deploy.yml</span>
                <Button size="sm" variant="ghost" className="text-xs">
                  Copy
                </Button>
              </div>
              <pre className="text-xs text-gray-300 overflow-x-auto">
{`name: Deploy to AWS

on:
  push:
    branches: [ main ]

permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    
    steps:
      - name: Checkout code
        uses: actions/checkout@v3
      
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          role-to-assume: arn:aws:iam::\${{ secrets.AWS_ACCOUNT_ID }}:role/github-actions-role
          aws-region: us-east-1
      
      - name: Login to ECR
        run: |
          aws ecr get-login-password | docker login --username AWS --password-stdin \${{ secrets.ECR_URI }}
      
      - name: Build and push Docker image
        run: |
          docker build -t my-app .
          docker tag my-app:latest \${{ secrets.ECR_URI }}/my-app:latest
          docker push \${{ secrets.ECR_URI }}/my-app:latest
      
      - name: Deploy to ECS
        run: |
          aws ecs update-service --cluster my-cluster --service my-service --force-new-deployment`}</pre>
            </div>
            
            <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
              <h4 className="text-sm font-medium text-blue-400 mb-2">Required Secrets</h4>
              <ul className="text-xs text-gray-400 space-y-1">
                <li>• <code className="text-blue-300">AWS_ACCOUNT_ID</code> - Your AWS account ID</li>
                <li>• <code className="text-blue-300">ECR_URI</code> - Your ECR repository URI</li>
              </ul>
            </div>

            <div className="bg-gray-800 rounded-lg p-3">
              <h4 className="text-sm font-medium text-gray-300 mb-2">IAM Trust Policy</h4>
              <pre className="text-xs text-gray-400 overflow-x-auto">
{`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {
      "Federated": "arn:aws:iam::\${AWS_ACCOUNT_ID}:oidc-provider/token.actions.githubusercontent.com"
    },
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringEquals": {
        "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
      },
      "StringLike": {
        "token.actions.githubusercontent.com:sub": "repo:Owner/Repo:*"
      }
    }
  }]
}`}</pre>
            </div>
          </div>
        )}

        {activeTab === 'repos' && selectedNode.type === 'ecr' && config && (
          <ECRRepositoryList config={config} accountInfo={accountInfo} />
        )}

        {activeTab === 'push' && selectedNode.type === 'ecr' && config && (
          <ECRPushInstructions config={config} accountInfo={accountInfo} />
        )}

        {activeTab === 'dns' && selectedNode.type === 'route53' && config && (
          <Route53DNSRecords config={config} />
        )}

        {activeTab === 'description' && selectedNode.type === 'secrets-manager' && config && (
          <ParameterStoreDescription config={config} />
        )}

        {activeTab === 'status' && selectedNode.type === 'ses' && config && (
          <SESStatus config={config} />
        )}

        {activeTab === 'send' && selectedNode.type === 'ses' && config && (
          <SESSendTestEmail config={config} />
        )}

        {activeTab === 'scaling' && selectedNode.type === 'backend' && config && onConfigChange && (
          <BackendScalingConfiguration config={config} onConfigChange={onConfigChange} />
        )}

        {activeTab === 'xray' && selectedNode.type === 'backend' && config && onConfigChange && (
          <BackendXRayConfiguration config={config} onConfigChange={onConfigChange} />
        )}

        {activeTab === 'ssh' && selectedNode.type === 'backend' && config && onConfigChange && (
          <BackendSSHAccess config={config} onConfigChange={onConfigChange} accountInfo={accountInfo} />
        )}

        {activeTab === 'env' && selectedNode.type === 'backend' && config && (
          <BackendEnvironmentVariables config={config} accountInfo={accountInfo} />
        )}

        {activeTab === 'params' && selectedNode.type === 'backend' && config && (
          <BackendParameterStore config={config} />
        )}

        {activeTab === 's3' && selectedNode.type === 'backend' && config && (
          <BackendS3Buckets config={config} />
        )}

        {activeTab === 'iam' && selectedNode.type === 'backend' && config && (
          <BackendIAMPermissions config={config} />
        )}

        {activeTab === 'cloudwatch' && selectedNode.type === 'backend' && config && (
          <BackendCloudWatch config={config} />
        )}

        {activeTab === 'alerts' && selectedNode.type === 'backend' && config && (
          <BackendAlerts config={config} />
        )}

        {/* Scheduled Task Tabs */}
        {activeTab === 'env' && selectedNode.type === 'scheduled-task' && config && (
          <div className="space-y-4">
            <h3 className="font-medium text-white">Environment Variables</h3>
            
            {/* Static Environment Variable */}
            <div className="space-y-3">
              <div className="bg-gray-800 rounded-lg p-4">
                <h4 className="text-sm font-medium text-gray-200 mb-3">Static Environment Variables</h4>
                {config.sqs?.enabled ? (
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-mono text-blue-400">SQS_QUEUE_URL</span>
                      <span className="text-xs text-gray-500">Only when SQS is enabled</span>
                    </div>
                    <div className="text-sm font-mono text-gray-300 break-all bg-gray-900 p-2 rounded">
                      https://sqs.{config.region}.amazonaws.com/{accountInfo?.accountId || '<ACCOUNT_ID>'}/{config.project}-{config.env}-{config.sqs.name || 'queue'}
                    </div>
                  </div>
                ) : (
                  <p className="text-sm text-gray-400">
                    No environment variables. Enable SQS to get <code className="text-blue-400">SQS_QUEUE_URL</code>.
                  </p>
                )}
              </div>
              
              {/* Note about other variables */}
              <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
                <p className="text-xs text-gray-300">
                  <strong className="text-blue-400">Note:</strong> To set custom environment variables, create parameters in AWS Systems Manager Parameter Store under:
                </p>
                <code className="text-xs text-gray-400 block mt-1">
                  /{config.env}/{config.project}/task/{selectedNode.id.replace('scheduled-', '')}/
                </code>
              </div>
            </div>
          </div>
        )}

        {activeTab === 'params' && selectedNode.type === 'scheduled-task' && config && (
          <ScheduledTaskParameterStore config={config} node={selectedNode} />
        )}

        {activeTab === 'iam' && selectedNode.type === 'scheduled-task' && config && (
          <ScheduledTaskIAMPermissions config={config} node={selectedNode} />
        )}

        {activeTab === 'cloudwatch' && selectedNode.type === 'scheduled-task' && config && (
          <ScheduledTaskCloudWatch config={config} node={selectedNode} />
        )}

        {activeTab === 'logs' && selectedNode.type === 'scheduled-task' && config && (
          <ServiceLogs 
            environment={config.env} 
            serviceName={selectedNode.id.replace('scheduled-', '')} 
          />
        )}

        {/* Event Task Tabs */}
        {activeTab === 'env' && selectedNode.type === 'event-task' && config && (
          <div className="space-y-4">
            <h3 className="font-medium text-white">Environment Variables</h3>
            
            {/* Static Environment Variable */}
            <div className="space-y-3">
              <div className="bg-gray-800 rounded-lg p-4">
                <h4 className="text-sm font-medium text-gray-200 mb-3">Static Environment Variables</h4>
                {config.sqs?.enabled ? (
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-mono text-blue-400">SQS_QUEUE_URL</span>
                      <span className="text-xs text-gray-500">Only when SQS is enabled</span>
                    </div>
                    <div className="text-sm font-mono text-gray-300 break-all bg-gray-900 p-2 rounded">
                      https://sqs.{config.region}.amazonaws.com/{accountInfo?.accountId || '<ACCOUNT_ID>'}/{config.project}-{config.env}-{config.sqs.name || 'queue'}
                    </div>
                  </div>
                ) : (
                  <p className="text-sm text-gray-400">
                    No environment variables. Enable SQS to get <code className="text-blue-400">SQS_QUEUE_URL</code>.
                  </p>
                )}
              </div>
              
              {/* Note about other variables */}
              <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
                <p className="text-xs text-gray-300">
                  <strong className="text-blue-400">Note:</strong> To set custom environment variables, create parameters in AWS Systems Manager Parameter Store under:
                </p>
                <code className="text-xs text-gray-400 block mt-1">
                  /{config.env}/{config.project}/task/{selectedNode.id.replace('event-', '')}/
                </code>
              </div>
            </div>
          </div>
        )}

        {activeTab === 'test' && selectedNode.type === 'event-task' && config && (
          <EventTaskTestEvent config={config} node={selectedNode} />
        )}

        {activeTab === 'params' && selectedNode.type === 'event-task' && config && (
          <ScheduledTaskParameterStore config={config} node={selectedNode} />
        )}

        {activeTab === 'iam' && selectedNode.type === 'event-task' && config && (
          <ScheduledTaskIAMPermissions config={config} node={selectedNode} />
        )}

        {activeTab === 'cloudwatch' && selectedNode.type === 'event-task' && config && (
          <ScheduledTaskCloudWatch config={config} node={selectedNode} />
        )}

        {activeTab === 'logs' && selectedNode.type === 'event-task' && config && (
          <ServiceLogs 
            environment={config.env} 
            serviceName={selectedNode.id.replace('event-', '')} 
          />
        )}


        {activeTab === 'cluster' && selectedNode.type === 'ecs' && config && (
          <ECSClusterInfo config={config} />
        )}

        {activeTab === 'network' && selectedNode.type === 'ecs' && config && (
          <ECSNetworkInfo config={config} />
        )}

        {activeTab === 'services' && selectedNode.type === 'ecs' && config && (
          <ECSServicesInfo config={config} />
        )}

        {activeTab === 'notifications' && selectedNode.type === 'ecs' && config && onConfigChange && (
          <ECSNotifications config={config} onConfigChange={onConfigChange} />
        )}
      </div>
    </div>
  );
}
import {
  Activity,
  BarChart,
  Bell,
  BookOpen,
  ChevronLeft,
  ChevronRight,
  Cloud,
  Code,
  Database,
  FileText,
  Gauge,
  GitBranch,
  Globe,
  HardDrive,
  Key,
  Link,
  Microscope,
  Network,
  Send,
  Server,
  Settings,
  Shield,
  Terminal,
  Trash2,
  Upload,
  Workflow,
  X,
  Zap,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { ALBNodeProperties } from "./ALBNodeProperties";
import { ALBRoutingRules } from "./ALBRoutingRules";
import { ALBStatus } from "./ALBStatus";
import { AmplifyBranchManagement } from "./AmplifyBranchManagement";
import { AmplifyBuildSettings } from "./AmplifyBuildSettings";
import { AmplifyDomainSettings } from "./AmplifyDomainSettings";
import { AmplifyEnvironmentVariables } from "./AmplifyEnvironmentVariables";
import { AmplifyNodeProperties } from "./AmplifyNodeProperties";
import { APIGatewayProperties } from "./APIGatewayProperties";
import { AuroraNodeProperties } from "./AuroraNodeProperties";
import { BackendAlerts } from "./BackendAlerts";
import { BackendCloudWatch } from "./BackendCloudWatch";
import { BackendEnvironmentVariables } from "./BackendEnvironmentVariables";
import { BackendIAMPermissions } from "./BackendIAMPermissions";
import { BackendParameterStore } from "./BackendParameterStore";
import { BackendS3Buckets } from "./BackendS3Buckets";
import { BackendScalingConfiguration } from "./BackendScalingConfiguration";
import { BackendServiceProperties } from "./BackendServiceProperties";
import { BackendSNS } from "./BackendSNS";
import { BackendSSHAccess } from "./BackendSSHAccess";
import { BackendXRayConfiguration } from "./BackendXRayConfiguration";
import { ECRNodeProperties } from "./ECRNodeProperties";
import { ECRPushInstructions } from "./ECRPushInstructions";
import { ECRRepositoryList } from "./ECRRepositoryList";
import {
  ECSClusterInfo,
  ECSNetworkInfo,
  ECSNodeProperties,
  ECSNotifications,
  ECSServicesInfo,
} from "./ECSNodeProperties";
import { EventBridgeTestEvent } from "./EventBridgeTestEvent";
import { EventTaskProperties } from "./EventTaskProperties";
import { EventTaskTestEvent } from "./EventTaskTestEvent";
import { GitHubNodeProperties } from "./GitHubNodeProperties";
import { NodeConfigProperties } from "./NodeConfigProperties";
import { ParameterStoreDescription } from "./ParameterStoreDescription";
import { ParameterStoreNodeProperties } from "./ParameterStoreNodeProperties";
import { PostgresConnectionInfo } from "./PostgresConnectionInfo";
import { PostgresNodeProperties } from "./PostgresNodeProperties";
import { Route53DNSRecords } from "./Route53DNSRecords";
import { Route53NodeProperties } from "./Route53NodeProperties";
import { S3NodeProperties } from "./S3NodeProperties";
import { ScheduledTaskCloudWatch } from "./ScheduledTaskCloudWatch";
import { ScheduledTaskIAMPermissions } from "./ScheduledTaskIAMPermissions";
import { ScheduledTaskParameterStore } from "./ScheduledTaskParameterStore";
import { ScheduledTaskProperties } from "./ScheduledTaskProperties";
import { SESNodeProperties } from "./SESNodeProperties";
import { SESSendTestEmail } from "./SESSendTestEmail";
import { SESStatus } from "./SESStatus";
import { ServiceEnvironmentVariables } from "./ServiceEnvironmentVariables";
import { ServiceLogs } from "./ServiceLogs";
import { ServiceParameterStore } from "./ServiceParameterStore";
import { ServiceProperties } from "./ServiceProperties";
import { ServiceXRayConfiguration } from "./ServiceXRayConfiguration";
import ServiceCICDConfiguration from "./ServiceCICDConfiguration";
import { SNSNodeProperties } from "./SNSNodeProperties";
import { SQSNodeProperties } from "./SQSNodeProperties";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";

interface SidebarProps {
  selectedNode: ComponentNode | null;
  isOpen: boolean;
  onClose: () => void;
  config?: YamlInfrastructureConfig;
  onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
  accountInfo?: AccountInfo;
  onDeleteNode?: (nodeId: string, nodeType: string) => void;
}

export function Sidebar({
  selectedNode,
  isOpen,
  onClose,
  config,
  onConfigChange,
  accountInfo,
  onDeleteNode,
}: SidebarProps) {
  const [activeTab, setActiveTab] = useState("settings");
  const [showLeftScroll, setShowLeftScroll] = useState(false);
  const [showRightScroll, setShowRightScroll] = useState(false);
  const tabsContainerRef = useRef<HTMLDivElement>(null);
  const tabButtonRefs = useRef<{ [key: string]: HTMLButtonElement | null }>({});

  // Function to get available tabs for the current node type
  const getAvailableTabs = useCallback((node: ComponentNode | null) => {
    if (!node) return [];

    switch (node.type) {
      case "github":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "example", label: "Example", icon: Code },
        ];
      case "ecr":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "repos", label: "Repos", icon: Database },
          { id: "push", label: "Push", icon: Upload },
        ];
      case "route53":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "dns", label: "DNS", icon: Globe },
        ];
      case "aurora":
        return [{ id: "settings", label: "Settings", icon: Settings }];
      case "secrets-manager":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "description", label: "Description", icon: BookOpen },
        ];
      case "ses":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "status", label: "Status", icon: Activity },
          { id: "send", label: "Send Email", icon: Send },
        ];
      case "s3":
        return [{ id: "settings", label: "Buckets", icon: HardDrive }];
      case "sns":
        return [{ id: "settings", label: "Settings", icon: Settings }];
      case "postgres":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "connection", label: "Connection", icon: Link },
        ];
      case "sqs":
        return [{ id: "settings", label: "Settings", icon: Settings }];
      case "eventbridge":
        return [{ id: "test", label: "Test Event", icon: Send }];
      case "api-gateway":
        return [{ id: "settings", label: "Settings", icon: Settings }];
      case "alb":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "routing", label: "Routing", icon: Network },
        ];
      case "ecs":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "notifications", label: "Notifications", icon: Bell },
          { id: "cluster", label: "Cluster", icon: Server },
          { id: "network", label: "Network", icon: Network },
          { id: "services", label: "Services", icon: Activity },
        ];
      case "backend":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "cicd", label: "CI/CD", icon: Workflow },
          { id: "scaling", label: "Scaling", icon: Gauge },
          { id: "xray", label: "X-Ray", icon: Microscope },
          { id: "ssh", label: "SSH", icon: Terminal },
          { id: "env", label: "Env Vars", icon: Zap },
          { id: "params", label: "Parameters", icon: Key },
          { id: "s3", label: "S3 Buckets", icon: HardDrive },
          { id: "sns", label: "SNS", icon: Bell },
          { id: "iam", label: "IAM", icon: Shield },
          { id: "logs", label: "Logs", icon: FileText },
          { id: "cloudwatch", label: "CloudWatch", icon: Cloud },
          { id: "alerts", label: "Alerts", icon: Bell },
        ];
      case "scheduled-task":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "cicd", label: "CI/CD", icon: Workflow },
          { id: "env", label: "Env Vars", icon: Zap },
          { id: "params", label: "Parameters", icon: Key },
          { id: "iam", label: "IAM", icon: Shield },
          { id: "cloudwatch", label: "CloudWatch", icon: Cloud },
          { id: "logs", label: "Logs", icon: FileText },
        ];
      case "event-task":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "cicd", label: "CI/CD", icon: Workflow },
          { id: "test", label: "Test Event", icon: Send },
          { id: "env", label: "Env Vars", icon: Zap },
          { id: "params", label: "Parameters", icon: Key },
          { id: "iam", label: "IAM", icon: Shield },
          { id: "cloudwatch", label: "CloudWatch", icon: Cloud },
          { id: "logs", label: "Logs", icon: FileText },
        ];
      case "service":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "cicd", label: "CI/CD", icon: Workflow },
          { id: "scaling", label: "Scaling", icon: Gauge },
          { id: "xray", label: "X-Ray", icon: Microscope },
          { id: "ssh", label: "SSH", icon: Terminal },
          { id: "env", label: "Env Vars", icon: Zap },
          { id: "params", label: "Parameters", icon: Key },
          { id: "iam", label: "IAM", icon: Shield },
          { id: "logs", label: "Logs", icon: FileText },
          { id: "cloudwatch", label: "CloudWatch", icon: Cloud },
        ];
      case "amplify":
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "cicd", label: "CI/CD", icon: Workflow },
          { id: "branches", label: "Branches", icon: GitBranch },
          { id: "build", label: "Build", icon: Code },
          { id: "domain", label: "Domain", icon: Globe },
          { id: "env", label: "Env Vars", icon: Zap },
        ];
      default:
        return [
          { id: "settings", label: "Settings", icon: Settings },
          { id: "logs", label: "Logs", icon: FileText },
          { id: "metrics", label: "Metrics", icon: BarChart },
          { id: "env", label: "Environment", icon: Zap },
          { id: "connections", label: "Connections", icon: Link },
        ];
    }
  }, []);

  // Effect to validate and update activeTab when selectedNode changes
  useEffect(() => {
    if (selectedNode) {
      const availableTabs = getAvailableTabs(selectedNode);
      const tabIds = availableTabs.map((tab) => tab.id);

      // If current activeTab doesn't exist for this node type, set to first available tab
      if (!tabIds.includes(activeTab) && availableTabs.length > 0) {
        setActiveTab(availableTabs[0].id);
      }
    }
  }, [selectedNode, activeTab, getAvailableTabs]);

  const checkScrollButtons = useCallback(() => {
    if (tabsContainerRef.current) {
      const { scrollLeft, scrollWidth, clientWidth } = tabsContainerRef.current;
      setShowLeftScroll(scrollLeft > 0);
      setShowRightScroll(scrollLeft < scrollWidth - clientWidth - 5);
    }
  }, []);

  useEffect(() => {
    checkScrollButtons();
    const container = tabsContainerRef.current;
    if (container) {
      container.addEventListener("scroll", checkScrollButtons);
      window.addEventListener("resize", checkScrollButtons);
      return () => {
        container.removeEventListener("scroll", checkScrollButtons);
        window.removeEventListener("resize", checkScrollButtons);
      };
    }
  }, [checkScrollButtons]);

  const scrollTabs = (direction: "left" | "right") => {
    if (tabsContainerRef.current) {
      const scrollAmount = 150;
      tabsContainerRef.current.scrollBy({
        left: direction === "left" ? -scrollAmount : scrollAmount,
        behavior: "smooth",
      });
      // Check scroll buttons after animation completes
      setTimeout(() => checkScrollButtons(), 350);
    }
  };

  // Function to center the active tab in view
  const centerTab = useCallback((tabId: string) => {
    const container = tabsContainerRef.current;
    const button = tabButtonRefs.current[tabId];

    if (container && button) {
      const containerWidth = container.clientWidth;
      const buttonLeft = button.offsetLeft;
      const buttonWidth = button.offsetWidth;

      // Calculate scroll position to center the button
      const scrollPosition = buttonLeft - (containerWidth / 2) + (buttonWidth / 2);

      container.scrollTo({
        left: scrollPosition,
        behavior: "smooth",
      });
      // Check scroll buttons after animation completes
      setTimeout(() => checkScrollButtons(), 350);
    }
  }, [checkScrollButtons]);

  // Auto-center the active tab when it changes
  useEffect(() => {
    if (activeTab) {
      // Small delay to ensure DOM is ready
      const timer = setTimeout(() => {
        centerTab(activeTab);
        // Don't call checkScrollButtons here - let the scroll event listener handle it
      }, 100);
      return () => clearTimeout(timer);
    }
  }, [activeTab, centerTab]);

  // Check scroll buttons when selectedNode changes (new tabs loaded)
  useEffect(() => {
    if (selectedNode) {
      // Delay to ensure tabs are rendered in DOM
      const timer = setTimeout(() => {
        checkScrollButtons();
      }, 150);
      return () => clearTimeout(timer);
    }
  }, [selectedNode, checkScrollButtons]);

  if (!isOpen || !selectedNode) return null;

  return (
    <div className="fixed right-0 top-0 h-full w-[768px] bg-gray-900 border-l border-gray-700 shadow-xl z-50 flex flex-col">
      <div className="flex items-start justify-between p-4 border-b border-gray-700 flex-shrink-0">
        <div className="flex-1 pr-2">
          <h2 className="text-lg font-medium text-white">
            {selectedNode.name}
          </h2>
          {config && (
            <div className="grid grid-cols-3 gap-2 mt-2">
              <div>
                <span className="text-xs text-gray-400 block">Project</span>
                <span className="text-xs text-gray-300 font-mono">
                  {config.project}
                </span>
              </div>
              <div>
                <span className="text-xs text-gray-400 block">Region</span>
                <span className="text-xs text-gray-300 font-mono">
                  {config.region}
                </span>
              </div>
              <div>
                <span className="text-xs text-gray-400 block">Environment</span>
                <span className="text-xs text-gray-300 font-mono">
                  {config.env}
                </span>
              </div>
            </div>
          )}
        </div>
        <div className="flex items-center gap-2">
          {(selectedNode.type === "service" ||
            selectedNode.type === "scheduled-task" ||
            selectedNode.type === "event-task" ||
            selectedNode.type === "amplify") &&
            onDeleteNode && (
              <Button
                variant="ghost"
                size="icon"
                onClick={() => {
                  if (
                    window.confirm(
                      `Are you sure you want to delete ${selectedNode.name}? This action cannot be undone.`,
                    )
                  ) {
                    onDeleteNode(selectedNode.id, selectedNode.type);
                    onClose();
                  }
                }}
                className="text-red-400 hover:text-red-300 hover:bg-red-900/20 flex-shrink-0"
                title="Delete"
              >
                <Trash2 className="w-4 h-4" />
              </Button>
            )}
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            className="text-gray-400 hover:text-white flex-shrink-0"
          >
            <X className="w-4 h-4" />
          </Button>
        </div>
      </div>

      <div className="relative flex-shrink-0 border-b border-gray-700">
        {/* Left scroll button - always visible */}
        <button
          type="button"
          onClick={() => scrollTabs("left")}
          disabled={!showLeftScroll}
          className={`absolute left-0 top-0 bottom-0 z-10 px-3 bg-gradient-to-r from-gray-900 via-gray-900 to-transparent flex items-center justify-center transition-all ${
            showLeftScroll
              ? "opacity-100 cursor-pointer hover:bg-gradient-to-r hover:from-gray-800"
              : "opacity-30 cursor-not-allowed"
          }`}
          aria-label="Scroll left"
        >
          <ChevronLeft className={`w-5 h-5 ${showLeftScroll ? "text-blue-400" : "text-gray-600"}`} />
        </button>

        {/* Right scroll button - always visible */}
        <button
          type="button"
          onClick={() => scrollTabs("right")}
          disabled={!showRightScroll}
          className={`absolute right-0 top-0 bottom-0 z-10 px-3 bg-gradient-to-l from-gray-900 via-gray-900 to-transparent flex items-center justify-center transition-all ${
            showRightScroll
              ? "opacity-100 cursor-pointer hover:bg-gradient-to-l hover:from-gray-800"
              : "opacity-30 cursor-not-allowed"
          }`}
          aria-label="Scroll right"
        >
          <ChevronRight className={`w-5 h-5 ${showRightScroll ? "text-blue-400" : "text-gray-600"}`} />
        </button>

        <div
          ref={tabsContainerRef}
          className="overflow-x-auto scrollbar-hide"
          style={{ scrollbarWidth: "none", msOverflowStyle: "none" }}
        >
          <div className="flex min-w-max">
            {getAvailableTabs(selectedNode).map((tab) => (
              <button
                type="button"
                key={tab.id}
                ref={(el) => {
                  tabButtonRefs.current[tab.id] = el;
                }}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center justify-center gap-2 px-4 py-2 text-sm font-medium transition-colors whitespace-nowrap ${
                  activeTab === tab.id
                    ? "text-blue-400 border-b-2 border-blue-400"
                    : "text-gray-400 hover:text-white"
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
        {/* Special handling for postgres with connection tab */}
        {selectedNode.type === "postgres" && config && onConfigChange ? (
          activeTab === "connection" ? (
            <PostgresConnectionInfo config={config} />
          ) : activeTab === "settings" ? (
            <PostgresNodeProperties
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
            />
          ) : null
        ) : (
          activeTab === "settings" &&
          (selectedNode.type === "ecs" && config && onConfigChange ? (
            <ECSNodeProperties
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === "backend" && config && onConfigChange ? (
            <BackendServiceProperties
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
            />
          ) : selectedNode.type === "github" && config && onConfigChange ? (
            <GitHubNodeProperties
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === "ecr" && config && onConfigChange ? (
            <ECRNodeProperties
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
            />
          ) : selectedNode.type === "route53" && config && onConfigChange ? (
            <Route53NodeProperties
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === "api-gateway" && config ? (
            <APIGatewayProperties config={config} />
          ) : selectedNode.type === "alb" && config && onConfigChange ? (
            <ALBNodeProperties
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === "aurora" ? (
            <AuroraNodeProperties />
          ) : selectedNode.type === "secrets-manager" && config ? (
            <ParameterStoreNodeProperties config={config} />
          ) : selectedNode.type === "ses" && config && onConfigChange ? (
            <SESNodeProperties
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === "s3" && config && onConfigChange ? (
            <S3NodeProperties config={config} onConfigChange={onConfigChange} />
          ) : selectedNode.type === "sns" && config && onConfigChange ? (
            <SNSNodeProperties
              config={config}
              onConfigChange={onConfigChange}
            />
          ) : selectedNode.type === "sqs" && config && onConfigChange ? (
            <SQSNodeProperties
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
            />
          ) : selectedNode.type === "scheduled-task" &&
            config &&
            onConfigChange ? (
            <ScheduledTaskProperties
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
              node={selectedNode}
            />
          ) : selectedNode.type === "event-task" && config && onConfigChange ? (
            <EventTaskProperties
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
              node={selectedNode}
            />
          ) : selectedNode.type === "service" && config && onConfigChange ? (
            <ServiceProperties
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
              node={selectedNode}
            />
          ) : selectedNode.type === "amplify" && config && onConfigChange ? (
            <AmplifyNodeProperties
              config={config}
              nodeId={selectedNode.id}
              onConfigChange={onConfigChange}
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
                  value={selectedNode.url || "Not available"}
                  className="mt-1 bg-gray-800 border-gray-600 text-white"
                  readOnly
                />
              </div>

              <div>
                <Label>Status</Label>
                <div className="mt-1 px-3 py-2 bg-gray-800 border border-gray-600 rounded-md">
                  <span
                    className={`capitalize ${
                      selectedNode.status === "running"
                        ? "text-green-400"
                        : selectedNode.status === "deploying"
                          ? "text-yellow-400"
                          : selectedNode.status === "error"
                            ? "text-red-400"
                            : "text-gray-400"
                    }`}
                  >
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
          ))
        )}

        {activeTab === "logs" &&
          config &&
          (selectedNode.type === "backend" ? (
            <ServiceLogs environment={config.env} serviceName="backend" />
          ) : selectedNode.type === "service" ? (
            <ServiceLogs
              environment={config.env}
              serviceName={selectedNode.id.replace("service-", "")}
            />
          ) : null)}

        {activeTab === "metrics" && (
          <div className="space-y-6">
            <div>
              <h3 className="font-medium text-white mb-4">
                Performance Metrics
              </h3>
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

        {activeTab === "connections" && (
          <div className="space-y-4">
            <h3 className="font-medium text-white">Connected Services</h3>
            <div className="space-y-2">
              {["database", "cache", "api-gateway"].map((service) => (
                <div
                  key={service}
                  className="bg-gray-800 p-3 rounded-lg flex items-center gap-3"
                >
                  <div className="w-2 h-2 bg-green-400 rounded-full"></div>
                  <span className="text-white">{service}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {activeTab === "example" && selectedNode.type === "github" && (
          <div className="space-y-4">
            <h3 className="font-medium text-white mb-4">
              GitHub Actions Workflow Example
            </h3>
            <div className="bg-gray-800 rounded-lg p-4">
              <div className="flex items-center justify-between mb-3">
                <span className="text-sm text-gray-400 font-mono">
                  .github/workflows/deploy.yml
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-xs"
                  onClick={() => {
                    const accountId =
                      accountInfo?.accountId ||
                      config?.account_id ||
                      "123456789012";

                    // Generate different workflow based on ECR strategy
                    const scriptContent = config?.ecr_strategy === "cross_account"
                      ? `name: Deploy to AWS (${config?.env || "dev"})

on:
  workflow_dispatch: # Manual triggering enabled

env:
  AWS_REGION: ${config?.region || "us-east-1"}
  AWS_ACCOUNT_ID: "${accountId}"

permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4.0.1
        with:
          role-to-assume: arn:aws:iam::\${{ env.AWS_ACCOUNT_ID }}:role/${config?.project || "project"}-${config?.env || "dev"}-github-actions-role
          role-session-name: github-actions-deploy
          aws-region: \${{ env.AWS_REGION }}

      - name: Trigger deployment via EventBridge
        run: |
          aws events put-events --entries 'Source=action.${config?.env || "dev"},DetailType=DEPLOY,Detail="{\\"service\\":\\"backend\\"}",EventBusName=default'`
                      : `name: Deploy to AWS (${config?.env || "dev"})

on:
  push:
    branches: [ main ]

concurrency:
  group: deploy-\${{ github.ref }}
  cancel-in-progress: true

env:
  AWS_REGION: ${config?.region || "us-east-1"}
  AWS_ACCOUNT_ID: ${accountId}
  ECR_REPOSITORY: ${config?.project || "project"}-${config?.env || "dev"}-backend
  ECS_CLUSTER: ${config?.project || "project"}-${config?.env || "dev"}-cluster
  ECS_SERVICE: ${config?.project || "project"}-${config?.env || "dev"}-backend
  IAM_ROLE: arn:aws:iam::${accountId}:role/${config?.project || "project"}-${config?.env || "dev"}-github-actions-role

permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          role-to-assume: \${{ env.IAM_ROLE }}
          aws-region: \${{ env.AWS_REGION }}

      - name: Compute ECR registry
        id: ecr
        run: echo "ECR_REGISTRY=\${{ env.AWS_ACCOUNT_ID }}.dkr.ecr.\${{ env.AWS_REGION }}.amazonaws.com" >> "\$GITHUB_ENV"

      - name: Ensure ECR repository exists
        run: |
          aws ecr describe-repositories --repository-names "\$ECR_REPOSITORY" >/dev/null 2>&1 || \\
          aws ecr create-repository --repository-name "\$ECR_REPOSITORY" >/dev/null

      - name: Login to ECR
        run: |
          aws ecr get-login-password --region \$AWS_REGION | \\
          docker login --username AWS --password-stdin \$ECR_REGISTRY

      - name: Build and push Docker image
        run: |
          docker build -t \$ECR_REPOSITORY .
          docker tag \$ECR_REPOSITORY:latest \$ECR_REGISTRY/\$ECR_REPOSITORY:latest
          docker tag \$ECR_REPOSITORY:latest \$ECR_REGISTRY/\$ECR_REPOSITORY:\${{ github.sha }}
          docker push \$ECR_REGISTRY/\$ECR_REPOSITORY:latest
          docker push \$ECR_REGISTRY/\$ECR_REPOSITORY:\${{ github.sha }}

      - name: Deploy to ECS (force new deployment)
        run: |
          aws ecs update-service \\
            --cluster "\$ECS_CLUSTER" \\
            --service "\$ECS_SERVICE" \\
            --force-new-deployment \\
            --region "\$AWS_REGION"
          aws ecs wait services-stable --cluster "\$ECS_CLUSTER" --services "\$ECS_SERVICE"`;

                    navigator.clipboard.writeText(scriptContent);
                  }}
                >
                  Copy
                </Button>
              </div>
              <pre className="text-xs text-gray-300 overflow-x-auto">
                {(() => {
                  const accountId =
                    accountInfo?.accountId ||
                    config?.account_id ||
                    "123456789012";

                  // Generate different workflow based on ECR strategy
                  if (config?.ecr_strategy === "cross_account") {
                    return `name: Deploy to AWS (${config?.env || "dev"})

on:
  workflow_dispatch: # Manual triggering enabled

env:
  AWS_REGION: ${config?.region || "us-east-1"}
  AWS_ACCOUNT_ID: "${accountId}"

permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4.0.1
        with:
          role-to-assume: arn:aws:iam::\${{ env.AWS_ACCOUNT_ID }}:role/${config?.project || "project"}-${config?.env || "dev"}-github-actions-role
          role-session-name: github-actions-deploy
          aws-region: \${{ env.AWS_REGION }}

      - name: Trigger deployment via EventBridge
        run: |
          aws events put-events --entries 'Source=action.${config?.env || "dev"},DetailType=DEPLOY,Detail="{\\"service\\":\\"backend\\"}",EventBusName=default'`;
                  }

                  return `name: Deploy to AWS (${config?.env || "dev"})

on:
  push:
    branches: [ main ]

concurrency:
  group: deploy-\${{ github.ref }}
  cancel-in-progress: true

env:
  AWS_REGION: ${config?.region || "us-east-1"}
  AWS_ACCOUNT_ID: ${accountId}
  ECR_REPOSITORY: ${config?.project || "project"}-${config?.env || "dev"}-backend
  ECS_CLUSTER: ${config?.project || "project"}-${config?.env || "dev"}-cluster
  ECS_SERVICE: ${config?.project || "project"}-${config?.env || "dev"}-backend
  IAM_ROLE: arn:aws:iam::${accountId}:role/${config?.project || "project"}-${config?.env || "dev"}-github-actions-role

permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          role-to-assume: \${{ env.IAM_ROLE }}
          aws-region: \${{ env.AWS_REGION }}

      - name: Compute ECR registry
        id: ecr
        run: echo "ECR_REGISTRY=\${{ env.AWS_ACCOUNT_ID }}.dkr.ecr.\${{ env.AWS_REGION }}.amazonaws.com" >> "\$GITHUB_ENV"

      - name: Ensure ECR repository exists
        run: |
          aws ecr describe-repositories --repository-names "\$ECR_REPOSITORY" >/dev/null 2>&1 || \\
          aws ecr create-repository --repository-name "\$ECR_REPOSITORY" >/dev/null

      - name: Login to ECR
        run: |
          aws ecr get-login-password --region \$AWS_REGION | \\
          docker login --username AWS --password-stdin \$ECR_REGISTRY

      - name: Build and push Docker image
        run: |
          docker build -t \$ECR_REPOSITORY .
          docker tag \$ECR_REPOSITORY:latest \$ECR_REGISTRY/\$ECR_REPOSITORY:latest
          docker tag \$ECR_REPOSITORY:latest \$ECR_REGISTRY/\$ECR_REPOSITORY:\${{ github.sha }}
          docker push \$ECR_REGISTRY/\$ECR_REPOSITORY:latest
          docker push \$ECR_REGISTRY/\$ECR_REPOSITORY:\${{ github.sha }}

      - name: Deploy to ECS (force new deployment)
        run: |
          aws ecs update-service \\
            --cluster "\$ECS_CLUSTER" \\
            --service "\$ECS_SERVICE" \\
            --force-new-deployment \\
            --region "\$AWS_REGION"
          aws ecs wait services-stable --cluster "\$ECS_CLUSTER" --services "\$ECS_SERVICE"`;
                })()}
              </pre>
            </div>

            {(() => {
              const accountId =
                accountInfo?.accountId || config?.ecr_account_id;
              return accountId ? (
                <div className="bg-green-900/20 border border-green-700 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-green-400 mb-2">
                    AWS Account Configuration
                  </h4>
                  <ul className="text-xs text-gray-400 space-y-1">
                    <li>
                      • <code className="text-green-300">AWS Account ID</code>:{" "}
                      {accountId}
                    </li>
                    <li>
                      • <code className="text-green-300">Region</code>:{" "}
                      {config?.region || "us-east-1"}
                    </li>
                  </ul>
                </div>
              ) : (
                <div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-yellow-400 mb-2">
                    Configuration Required
                  </h4>
                  <p className="text-xs text-gray-400">
                    AWS Account ID not detected. Deploy infrastructure first to
                    populate account information.
                  </p>
                </div>
              );
            })()}

            {config?.ecr_strategy === "cross_account" ? (
              // Cross-account ECR resources
              <>
                <div className="bg-green-900/20 border border-green-700 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-green-400 mb-2">
                    Generated Resources
                  </h4>
                  <ul className="text-xs text-gray-400 space-y-1">
                    <li>
                      • <code className="text-green-300">ECS Cluster</code>:{" "}
                      {config?.project || "project"}-{config?.env || "dev"}-cluster
                    </li>
                    <li>
                      • <code className="text-green-300">ECS Service</code>:{" "}
                      {config?.project || "project"}-{config?.env || "dev"}-backend
                    </li>
                    <li>
                      • <code className="text-green-300">IAM Role</code>:{" "}
                      {config?.project || "project"}-{config?.env || "dev"}
                      -github-actions-role
                    </li>
                    <li>
                      • <code className="text-green-300">EventBridge</code>: Default event bus
                    </li>
                  </ul>
                </div>

                <div className="bg-purple-900/20 border border-purple-700 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-purple-400 mb-2">
                    Cross-Account Configuration
                  </h4>
                  <ul className="text-xs text-gray-400 space-y-1">
                    <li>
                      • <code className="text-purple-300">ECR Strategy</code>: Cross-Account
                    </li>
                    <li>
                      • <code className="text-purple-300">Source Account</code>:{" "}
                      {config?.ecr_account_id || "Not configured"}
                    </li>
                    <li>
                      • <code className="text-purple-300">Source Region</code>:{" "}
                      {config?.ecr_account_region || "Not configured"}
                    </li>
                  </ul>
                </div>

                <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-blue-400 mb-2">
                    Workflow Features
                  </h4>
                  <ul className="text-xs text-gray-400 space-y-1">
                    <li>
                      ✓ <span className="text-blue-300">Manual deployment</span> -
                      Trigger via workflow_dispatch
                    </li>
                    <li>
                      ✓ <span className="text-blue-300">EventBridge integration</span>{" "}
                      - Event-driven deployments
                    </li>
                    <li>
                      ✓ <span className="text-blue-300">Cross-account ECR</span> -
                      Uses images from source account
                    </li>
                    <li>
                      ✓ <span className="text-blue-300">OIDC authentication</span> -
                      Passwordless AWS access
                    </li>
                    <li>
                      ✓ <span className="text-blue-300">Lightweight deployment</span> -
                      No build or push steps
                    </li>
                  </ul>
                </div>

                <div className="bg-gray-800 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-gray-300 mb-2">
                    Important Notes
                  </h4>
                  <ul className="text-xs text-gray-400 space-y-1">
                    <li>
                      • This environment pulls container images from account{" "}
                      <code>{config?.ecr_account_id}</code>
                    </li>
                    <li>
                      • Images are built and pushed in the source environment
                    </li>
                    <li>
                      • Deployment is triggered manually or via EventBridge events
                    </li>
                    <li>
                      • Ensure cross-account ECR trust policies are deployed in source account
                    </li>
                  </ul>
                </div>
              </>
            ) : (
              // Local ECR resources (default)
              <>
                <div className="bg-green-900/20 border border-green-700 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-green-400 mb-2">
                    Generated Resources
                  </h4>
                  <ul className="text-xs text-gray-400 space-y-1">
                    <li>
                      • <code className="text-green-300">ECR Repository</code>:{" "}
                      {config?.project || "project"}-{config?.env || "dev"}-backend
                    </li>
                    <li>
                      • <code className="text-green-300">ECS Cluster</code>:{" "}
                      {config?.project || "project"}-{config?.env || "dev"}-cluster
                    </li>
                    <li>
                      • <code className="text-green-300">ECS Service</code>:{" "}
                      {config?.project || "project"}-{config?.env || "dev"}-backend
                    </li>
                    <li>
                      • <code className="text-green-300">IAM Role</code>:{" "}
                      {config?.project || "project"}-{config?.env || "dev"}
                      -github-actions-role
                    </li>
                  </ul>
                </div>

                <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-blue-400 mb-2">
                    Workflow Features
                  </h4>
                  <ul className="text-xs text-gray-400 space-y-1">
                    <li>
                      ✓ <span className="text-blue-300">Concurrency control</span> -
                      Prevents overlapping deployments
                    </li>
                    <li>
                      ✓ <span className="text-blue-300">Auto-create ECR repo</span>{" "}
                      - Creates repository if it doesn't exist
                    </li>
                    <li>
                      ✓ <span className="text-blue-300">Dual tagging</span> - Tags
                      with both :latest and :sha for rollback
                    </li>
                    <li>
                      ✓{" "}
                      <span className="text-blue-300">Deployment verification</span>{" "}
                      - Waits for service stability
                    </li>
                    <li>
                      ✓ <span className="text-blue-300">OIDC authentication</span> -
                      Passwordless AWS access
                    </li>
                  </ul>
                </div>

                <div className="bg-gray-800 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-gray-300 mb-2">
                    Important Notes
                  </h4>
                  <ul className="text-xs text-gray-400 space-y-1">
                    <li>
                      • Ensure your ECS task definition uses the{" "}
                      <code>:latest</code> tag for auto-updates
                    </li>
                    <li>
                      • The workflow will cancel any in-progress deployments when a
                      new push occurs
                    </li>
                    <li>
                      • Service stability check will fail the workflow if deployment
                      fails
                    </li>
                  </ul>
                </div>
              </>
            )}

            <div className="bg-gray-800 rounded-lg p-3">
              <h4 className="text-sm font-medium text-gray-300 mb-2">
                IAM Trust Policy
              </h4>
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
}`}
              </pre>
            </div>
          </div>
        )}

        {activeTab === "repos" && selectedNode.type === "ecr" && config && (
          <ECRRepositoryList config={config} accountInfo={accountInfo} />
        )}

        {activeTab === "push" && selectedNode.type === "ecr" && config && (
          <ECRPushInstructions config={config} accountInfo={accountInfo} />
        )}

        {activeTab === "dns" && selectedNode.type === "route53" && config && (
          <Route53DNSRecords config={config} />
        )}

        {activeTab === "description" &&
          selectedNode.type === "secrets-manager" &&
          config && <ParameterStoreDescription config={config} />}

        {activeTab === "status" && selectedNode.type === "ses" && config && (
          <SESStatus config={config} />
        )}

        {activeTab === "status" && selectedNode.type === "alb" && config && (
          <ALBStatus config={config} />
        )}

        {activeTab === "routing" &&
          selectedNode.type === "alb" &&
          config &&
          onConfigChange && (
            <ALBRoutingRules config={config} onConfigChange={onConfigChange} />
          )}

        {activeTab === "send" && selectedNode.type === "ses" && config && (
          <SESSendTestEmail config={config} />
        )}

        {/* CI/CD tabs for service nodes */}
        {activeTab === "cicd" &&
          selectedNode.type === "backend" &&
          config && (
            <ServiceCICDConfiguration
              config={config}
              serviceName="backend"
              serviceType="backend"
            />
          )}

        {activeTab === "cicd" &&
          selectedNode.type === "service" &&
          config && (
            <ServiceCICDConfiguration
              config={config}
              serviceName={selectedNode.id.replace("service-", "")}
              serviceType="service"
            />
          )}

        {activeTab === "cicd" &&
          selectedNode.type === "scheduled-task" &&
          config && (
            <ServiceCICDConfiguration
              config={config}
              serviceName={selectedNode.id.replace("scheduled-", "")}
              serviceType="scheduled-task"
            />
          )}

        {activeTab === "cicd" &&
          selectedNode.type === "event-task" &&
          config && (
            <ServiceCICDConfiguration
              config={config}
              serviceName={selectedNode.id.replace("event-", "")}
              serviceType="event-task"
            />
          )}

        {activeTab === "cicd" &&
          selectedNode.type === "amplify" &&
          config && (
            <ServiceCICDConfiguration
              config={config}
              serviceName={selectedNode.id.replace("amplify-", "")}
              serviceType="amplify"
            />
          )}

        {activeTab === "scaling" &&
          selectedNode.type === "backend" &&
          config &&
          onConfigChange && (
            <BackendScalingConfiguration
              config={config}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "xray" &&
          selectedNode.type === "backend" &&
          config &&
          onConfigChange && (
            <BackendXRayConfiguration
              config={config}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "ssh" &&
          selectedNode.type === "backend" &&
          config &&
          onConfigChange && (
            <BackendSSHAccess
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
            />
          )}

        {activeTab === "env" &&
          selectedNode.type === "backend" &&
          config &&
          onConfigChange && (
            <BackendEnvironmentVariables
              config={config}
              accountInfo={accountInfo}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "params" &&
          selectedNode.type === "backend" &&
          config && <BackendParameterStore config={config} />}

        {activeTab === "s3" &&
          selectedNode.type === "backend" &&
          config &&
          onConfigChange && (
            <BackendS3Buckets config={config} onConfigChange={onConfigChange} />
          )}

        {activeTab === "sns" && selectedNode.type === "backend" && config && (
          <BackendSNS config={config} />
        )}

        {activeTab === "iam" &&
          selectedNode.type === "backend" &&
          config &&
          onConfigChange && (
            <BackendIAMPermissions
              config={config}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "cloudwatch" &&
          selectedNode.type === "backend" &&
          config && <BackendCloudWatch config={config} />}

        {activeTab === "alerts" &&
          selectedNode.type === "backend" &&
          config && <BackendAlerts config={config} />}

        {/* Service-specific tabs */}
        {activeTab === "scaling" &&
          selectedNode.type === "service" &&
          config &&
          onConfigChange && (
            <BackendScalingConfiguration
              config={config}
              onConfigChange={onConfigChange}
              isService={true}
              serviceName={selectedNode.id.replace("service-", "")}
            />
          )}

        {activeTab === "xray" &&
          selectedNode.type === "service" &&
          config &&
          onConfigChange && (
            <ServiceXRayConfiguration
              config={config}
              onConfigChange={onConfigChange}
              node={selectedNode}
            />
          )}

        {activeTab === "ssh" &&
          selectedNode.type === "service" &&
          config &&
          onConfigChange && (
            <BackendSSHAccess
              config={config}
              onConfigChange={onConfigChange}
              accountInfo={accountInfo}
              isService={true}
              serviceName={selectedNode.id.replace("service-", "")}
            />
          )}

        {activeTab === "env" &&
          selectedNode.type === "service" &&
          config &&
          onConfigChange && (
            <ServiceEnvironmentVariables
              config={config}
              accountInfo={accountInfo}
              node={selectedNode}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "params" &&
          selectedNode.type === "service" &&
          config && (
            <ServiceParameterStore config={config} node={selectedNode} />
          )}

        {activeTab === "iam" &&
          selectedNode.type === "service" &&
          config &&
          onConfigChange && (
            <BackendIAMPermissions
              config={config}
              node={selectedNode}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "cloudwatch" &&
          selectedNode.type === "service" &&
          config && <BackendCloudWatch config={config} node={selectedNode} />}

        {/* Scheduled Task Tabs */}
        {activeTab === "env" &&
          selectedNode.type === "scheduled-task" &&
          config && (
            <div className="space-y-4">
              <h3 className="font-medium text-white">Environment Variables</h3>

              {/* Static Environment Variable */}
              <div className="space-y-3">
                <div className="bg-gray-800 rounded-lg p-4">
                  <h4 className="text-sm font-medium text-gray-200 mb-3">
                    Static Environment Variables
                  </h4>
                  {config.sqs?.enabled ? (
                    <div>
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-sm font-mono text-blue-400">
                          SQS_QUEUE_URL
                        </span>
                        <span className="text-xs text-gray-500">
                          Only when SQS is enabled
                        </span>
                      </div>
                      <div className="text-sm font-mono text-gray-300 break-all bg-gray-900 p-2 rounded">
                        https://sqs.{config.region}.amazonaws.com/
                        {accountInfo?.accountId || "<ACCOUNT_ID>"}/
                        {config.project}-{config.env}-
                        {config.sqs.name || "queue"}
                      </div>
                    </div>
                  ) : (
                    <p className="text-sm text-gray-400">
                      No environment variables. Enable SQS to get{" "}
                      <code className="text-blue-400">SQS_QUEUE_URL</code>.
                    </p>
                  )}
                </div>

                {/* Note about other variables */}
                <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
                  <p className="text-xs text-gray-300">
                    <strong className="text-blue-400">Note:</strong> To set
                    custom environment variables, create parameters in AWS
                    Systems Manager Parameter Store under:
                  </p>
                  <code className="text-xs text-gray-400 block mt-1">
                    /{config.env}/{config.project}/task/
                    {selectedNode.id.replace("scheduled-", "")}/
                  </code>
                </div>
              </div>
            </div>
          )}

        {activeTab === "params" &&
          selectedNode.type === "scheduled-task" &&
          config && (
            <ScheduledTaskParameterStore config={config} node={selectedNode} />
          )}

        {activeTab === "iam" &&
          selectedNode.type === "scheduled-task" &&
          config && (
            <ScheduledTaskIAMPermissions config={config} node={selectedNode} />
          )}

        {activeTab === "cloudwatch" &&
          selectedNode.type === "scheduled-task" &&
          config && (
            <ScheduledTaskCloudWatch config={config} node={selectedNode} />
          )}

        {activeTab === "logs" &&
          selectedNode.type === "scheduled-task" &&
          config && (
            <ServiceLogs
              environment={config.env}
              serviceName={selectedNode.id.replace("scheduled-", "")}
            />
          )}

        {/* Event Task Tabs */}
        {activeTab === "env" &&
          selectedNode.type === "event-task" &&
          config && (
            <div className="space-y-4">
              <h3 className="font-medium text-white">Environment Variables</h3>

              {/* Static Environment Variable */}
              <div className="space-y-3">
                <div className="bg-gray-800 rounded-lg p-4">
                  <h4 className="text-sm font-medium text-gray-200 mb-3">
                    Static Environment Variables
                  </h4>
                  {config.sqs?.enabled ? (
                    <div>
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-sm font-mono text-blue-400">
                          SQS_QUEUE_URL
                        </span>
                        <span className="text-xs text-gray-500">
                          Only when SQS is enabled
                        </span>
                      </div>
                      <div className="text-sm font-mono text-gray-300 break-all bg-gray-900 p-2 rounded">
                        https://sqs.{config.region}.amazonaws.com/
                        {accountInfo?.accountId || "<ACCOUNT_ID>"}/
                        {config.project}-{config.env}-
                        {config.sqs.name || "queue"}
                      </div>
                    </div>
                  ) : (
                    <p className="text-sm text-gray-400">
                      No environment variables. Enable SQS to get{" "}
                      <code className="text-blue-400">SQS_QUEUE_URL</code>.
                    </p>
                  )}
                </div>

                {/* Note about other variables */}
                <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
                  <p className="text-xs text-gray-300">
                    <strong className="text-blue-400">Note:</strong> To set
                    custom environment variables, create parameters in AWS
                    Systems Manager Parameter Store under:
                  </p>
                  <code className="text-xs text-gray-400 block mt-1">
                    /{config.env}/{config.project}/task/
                    {selectedNode.id.replace("event-", "")}/
                  </code>
                </div>
              </div>
            </div>
          )}

        {activeTab === "test" &&
          selectedNode.type === "event-task" &&
          config && <EventTaskTestEvent config={config} node={selectedNode} />}

        {activeTab === "test" &&
          selectedNode.type === "eventbridge" &&
          config && <EventBridgeTestEvent config={config} />}

        {activeTab === "params" &&
          selectedNode.type === "event-task" &&
          config && (
            <ScheduledTaskParameterStore config={config} node={selectedNode} />
          )}

        {activeTab === "iam" &&
          selectedNode.type === "event-task" &&
          config && (
            <ScheduledTaskIAMPermissions config={config} node={selectedNode} />
          )}

        {activeTab === "cloudwatch" &&
          selectedNode.type === "event-task" &&
          config && (
            <ScheduledTaskCloudWatch config={config} node={selectedNode} />
          )}

        {activeTab === "logs" &&
          selectedNode.type === "event-task" &&
          config && (
            <ServiceLogs
              environment={config.env}
              serviceName={selectedNode.id.replace("event-", "")}
            />
          )}

        {activeTab === "cluster" && selectedNode.type === "ecs" && config && (
          <ECSClusterInfo config={config} />
        )}

        {activeTab === "network" && selectedNode.type === "ecs" && config && (
          <ECSNetworkInfo config={config} />
        )}

        {activeTab === "services" && selectedNode.type === "ecs" && config && (
          <ECSServicesInfo config={config} />
        )}

        {activeTab === "notifications" &&
          selectedNode.type === "ecs" &&
          config &&
          onConfigChange && (
            <ECSNotifications config={config} onConfigChange={onConfigChange} />
          )}

        {/* Amplify tabs */}
        {activeTab === "build" &&
          selectedNode.type === "amplify" &&
          config &&
          onConfigChange && (
            <AmplifyBuildSettings
              config={config}
              nodeId={selectedNode.id}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "domain" &&
          selectedNode.type === "amplify" &&
          config &&
          onConfigChange && (
            <AmplifyDomainSettings
              config={config}
              nodeId={selectedNode.id}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "env" &&
          selectedNode.type === "amplify" &&
          config &&
          onConfigChange && (
            <AmplifyEnvironmentVariables
              config={config}
              nodeId={selectedNode.id}
              onConfigChange={onConfigChange}
            />
          )}

        {activeTab === "branches" &&
          selectedNode.type === "amplify" &&
          config &&
          onConfigChange && (
            <AmplifyBranchManagement
              config={config}
              nodeId={selectedNode.id}
              onConfigChange={onConfigChange}
            />
          )}
      </div>
    </div>
  );
}

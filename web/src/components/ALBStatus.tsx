import { useState, useEffect } from 'react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Activity, AlertCircle, CheckCircle, Loader2, RefreshCw } from 'lucide-react';
import { Button } from './ui/button';

interface ALBStatusProps {
  config: YamlInfrastructureConfig;
}

interface ALBHealthStatus {
  healthy: number;
  unhealthy: number;
  draining: number;
  initial: number;
  unused: number;
}

interface ALBMetrics {
  activeConnectionCount: number;
  newConnectionCount: number;
  processedBytes: number;
  requestCount: number;
  targetResponseTime: number;
}

export function ALBStatus({ config }: ALBStatusProps) {
  const [isLoading, setIsLoading] = useState(false);
  const [healthStatus, setHealthStatus] = useState<ALBHealthStatus | null>(null);
  const [metrics, setMetrics] = useState<ALBMetrics | null>(null);
  const [lastChecked, setLastChecked] = useState<Date | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchALBStatus = async () => {
    if (!config.alb?.enabled) return;

    setIsLoading(true);
    setError(null);

    try {
      // This would normally call an API endpoint to get ALB status
      // For now, we'll simulate the data
      await new Promise(resolve => setTimeout(resolve, 1000));

      // Simulated health status
      setHealthStatus({
        healthy: config.workload?.backend_desired_count || 2,
        unhealthy: 0,
        draining: 0,
        initial: 0,
        unused: 0
      });

      // Simulated metrics
      setMetrics({
        activeConnectionCount: Math.floor(Math.random() * 100) + 50,
        newConnectionCount: Math.floor(Math.random() * 20) + 5,
        processedBytes: Math.floor(Math.random() * 1000000) + 500000,
        requestCount: Math.floor(Math.random() * 1000) + 500,
        targetResponseTime: Math.random() * 0.5 + 0.1
      });

      setLastChecked(new Date());
    } catch (err) {
      setError('Failed to fetch ALB status');
      console.error('Error fetching ALB status:', err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchALBStatus();
    // Refresh every 60 seconds
    const interval = setInterval(fetchALBStatus, 60000);
    return () => clearInterval(interval);
  }, [config.alb?.enabled]);

  if (!config.alb?.enabled) {
    return (
      <Card>
        <CardContent className="pt-6">
          <div className="text-center text-gray-400">
            <AlertCircle className="w-12 h-12 mx-auto mb-2" />
            <p>ALB is not enabled</p>
          </div>
        </CardContent>
      </Card>
    );
  }

  const getTotalTargets = () => {
    if (!healthStatus) return 0;
    return healthStatus.healthy + healthStatus.unhealthy + healthStatus.draining + healthStatus.initial;
  };

  const getHealthPercentage = () => {
    const total = getTotalTargets();
    if (total === 0) return 0;
    return Math.round((healthStatus?.healthy || 0) / total * 100);
  };

  const formatBytes = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1048576).toFixed(1)} MB`;
  };

  return (
    <div className="space-y-4">
      {/* Health Status */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <Activity className="w-5 h-5" />
              Target Health Status
            </CardTitle>
            <Button
              size="sm"
              variant="ghost"
              onClick={fetchALBStatus}
              disabled={isLoading}
            >
              {isLoading ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : (
                <RefreshCw className="w-4 h-4" />
              )}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {error ? (
            <div className="text-red-400 text-sm flex items-center gap-2">
              <AlertCircle className="w-4 h-4" />
              {error}
            </div>
          ) : healthStatus ? (
            <div className="space-y-4">
              {/* Health Overview */}
              <div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className={`w-12 h-12 rounded-full flex items-center justify-center ${
                    getHealthPercentage() === 100 ? 'bg-green-500/20' : 
                    getHealthPercentage() >= 50 ? 'bg-yellow-500/20' : 'bg-red-500/20'
                  }`}>
                    {getHealthPercentage() === 100 ? (
                      <CheckCircle className="w-6 h-6 text-green-400" />
                    ) : (
                      <AlertCircle className="w-6 h-6 text-yellow-400" />
                    )}
                  </div>
                  <div>
                    <div className="font-medium text-white">
                      {getHealthPercentage()}% Healthy
                    </div>
                    <div className="text-sm text-gray-400">
                      {getTotalTargets()} total targets
                    </div>
                  </div>
                </div>
              </div>

              {/* Target Breakdown */}
              <div className="grid grid-cols-2 gap-3">
                <div className="bg-gray-800 rounded-lg p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400 text-sm">Healthy</span>
                    <span className="text-green-400 font-medium">{healthStatus.healthy}</span>
                  </div>
                </div>
                <div className="bg-gray-800 rounded-lg p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400 text-sm">Unhealthy</span>
                    <span className="text-red-400 font-medium">{healthStatus.unhealthy}</span>
                  </div>
                </div>
                <div className="bg-gray-800 rounded-lg p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400 text-sm">Draining</span>
                    <span className="text-yellow-400 font-medium">{healthStatus.draining}</span>
                  </div>
                </div>
                <div className="bg-gray-800 rounded-lg p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400 text-sm">Initial</span>
                    <span className="text-blue-400 font-medium">{healthStatus.initial}</span>
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="w-6 h-6 animate-spin text-gray-400" />
            </div>
          )}
        </CardContent>
      </Card>

      {/* Performance Metrics */}
      <Card>
        <CardHeader>
          <CardTitle>Performance Metrics</CardTitle>
        </CardHeader>
        <CardContent>
          {metrics ? (
            <div className="space-y-3">
              <div className="flex items-center justify-between py-2 border-b border-gray-700">
                <span className="text-gray-400 text-sm">Active Connections</span>
                <span className="font-mono text-white">{metrics.activeConnectionCount}</span>
              </div>
              <div className="flex items-center justify-between py-2 border-b border-gray-700">
                <span className="text-gray-400 text-sm">New Connections/min</span>
                <span className="font-mono text-white">{metrics.newConnectionCount}</span>
              </div>
              <div className="flex items-center justify-between py-2 border-b border-gray-700">
                <span className="text-gray-400 text-sm">Request Count (last hour)</span>
                <span className="font-mono text-white">{metrics.requestCount.toLocaleString()}</span>
              </div>
              <div className="flex items-center justify-between py-2 border-b border-gray-700">
                <span className="text-gray-400 text-sm">Data Processed</span>
                <span className="font-mono text-white">{formatBytes(metrics.processedBytes)}</span>
              </div>
              <div className="flex items-center justify-between py-2">
                <span className="text-gray-400 text-sm">Avg Response Time</span>
                <span className="font-mono text-white">{(metrics.targetResponseTime * 1000).toFixed(0)}ms</span>
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="w-6 h-6 animate-spin text-gray-400" />
            </div>
          )}
        </CardContent>
      </Card>

      {/* Last Updated */}
      {lastChecked && (
        <div className="text-xs text-gray-500 text-center">
          Last updated: {lastChecked.toLocaleTimeString()}
        </div>
      )}
    </div>
  );
}
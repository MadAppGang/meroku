import React, { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Alert, AlertDescription } from './ui/alert';
import { Button } from './ui/button';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { infrastructureApi, type SESStatusResponse as SESStatus, type SESSandboxInfo } from '../api/infrastructure';
import { 
  Activity,
  AlertCircle,
  CheckCircle,
  Info,
  Mail,
  RefreshCw,
  Shield,
  TrendingUp,
  XCircle,
  ExternalLink,
  Clock
} from 'lucide-react';

interface SESStatusProps {
  config: YamlInfrastructureConfig;
}

export function SESStatus({ config }: SESStatusProps) {
  const [status, setStatus] = useState<SESStatus | null>(null);
  const [sandboxInfo, setSandboxInfo] = useState<SESSandboxInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadStatus();
  }, []);

  const loadStatus = async () => {
    try {
      setLoading(true);
      setError(null);
      
      const [statusData, sandboxData] = await Promise.all([
        infrastructureApi.getSESStatus(),
        infrastructureApi.getSESSandboxInfo()
      ]);
      
      setStatus(statusData);
      setSandboxInfo(sandboxData);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load SES status');
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <RefreshCw className="w-6 h-6 animate-spin text-gray-400" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    );
  }

  if (!status) {
    return null;
  }

  return (
    <div className="space-y-4">
      {/* Account Status */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Activity className="w-5 h-5" />
                Account Status
              </CardTitle>
              <CardDescription>
                Current SES account status and sending capabilities
              </CardDescription>
            </div>
            <Button
              size="sm"
              variant="outline"
              onClick={loadStatus}
              disabled={loading}
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {/* Sandbox Status */}
            <div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
              <div className="flex items-center gap-3">
                <Shield className={`w-5 h-5 ${status.inSandbox ? 'text-yellow-400' : 'text-green-400'}`} />
                <div>
                  <p className="text-sm font-medium">Environment Mode</p>
                  <p className="text-xs text-gray-400">
                    {status.inSandbox ? 'Sandbox (Limited)' : 'Production (Full Access)'}
                  </p>
                </div>
              </div>
              <Badge variant={status.inSandbox ? 'warning' : 'success'}>
                {status.inSandbox ? 'Sandbox' : 'Production'}
              </Badge>
            </div>

            {/* Sending Status */}
            <div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
              <div className="flex items-center gap-3">
                {status.sendingEnabled ? (
                  <CheckCircle className="w-5 h-5 text-green-400" />
                ) : (
                  <XCircle className="w-5 h-5 text-red-400" />
                )}
                <div>
                  <p className="text-sm font-medium">Sending Status</p>
                  <p className="text-xs text-gray-400">
                    {status.sendingEnabled ? 'Email sending is enabled' : 'Email sending is disabled'}
                  </p>
                </div>
              </div>
              <Badge variant={status.sendingEnabled ? 'success' : 'destructive'}>
                {status.sendingEnabled ? 'Enabled' : 'Disabled'}
              </Badge>
            </div>

            {/* Quotas */}
            <div className="grid grid-cols-2 gap-3">
              <div className="p-3 bg-gray-800 rounded-lg">
                <div className="flex items-center gap-2 mb-1">
                  <Mail className="w-4 h-4 text-blue-400" />
                  <span className="text-xs text-gray-400">Daily Quota</span>
                </div>
                <p className="text-lg font-medium">{status.dailyQuota.toLocaleString()}</p>
                <p className="text-xs text-gray-500">emails per day</p>
              </div>
              
              <div className="p-3 bg-gray-800 rounded-lg">
                <div className="flex items-center gap-2 mb-1">
                  <TrendingUp className="w-4 h-4 text-blue-400" />
                  <span className="text-xs text-gray-400">Send Rate</span>
                </div>
                <p className="text-lg font-medium">{status.maxSendRate}</p>
                <p className="text-xs text-gray-500">emails per second</p>
              </div>
            </div>

            {/* 24h Stats */}
            <div className="p-3 bg-gray-800 rounded-lg">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Clock className="w-4 h-4 text-blue-400" />
                  <div>
                    <p className="text-sm font-medium">Last 24 Hours</p>
                    <p className="text-xs text-gray-400">Emails sent</p>
                  </div>
                </div>
                <p className="text-2xl font-medium">{status.sentLast24Hours}</p>
              </div>
            </div>

            {/* Reputation Status */}
            <div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
              <div className="flex items-center gap-3">
                <Shield className="w-4 h-4 text-blue-400" />
                <div>
                  <p className="text-sm font-medium">Reputation Status</p>
                  <p className="text-xs text-gray-400">Sender reputation health</p>
                </div>
              </div>
              <Badge 
                variant={
                  status.reputationStatus === 'Healthy' ? 'success' : 
                  status.reputationStatus === 'Default' ? 'secondary' : 
                  'warning'
                }
              >
                {status.reputationStatus}
              </Badge>
            </div>

            {/* Region */}
            <div className="flex items-center justify-between text-sm">
              <span className="text-gray-400">AWS Region</span>
              <span className="font-mono text-gray-300">{status.region}</span>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Verified Identities */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Mail className="w-5 h-5" />
            Verified Identities
          </CardTitle>
          <CardDescription>
            Domains and email addresses verified for sending
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {/* Verified Domains */}
            {status.verifiedDomains.length > 0 && (
              <div>
                <h4 className="text-sm font-medium text-gray-300 mb-2">Verified Domains</h4>
                <div className="space-y-2">
                  {status.verifiedDomains.map((domain, index) => (
                    <div key={index} className="flex items-center gap-2 p-2 bg-gray-800 rounded">
                      <CheckCircle className="w-4 h-4 text-green-400" />
                      <span className="text-sm font-mono">{domain}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Verified Emails */}
            {status.verifiedEmails.length > 0 && (
              <div>
                <h4 className="text-sm font-medium text-gray-300 mb-2">Verified Email Addresses</h4>
                <div className="space-y-2">
                  {status.verifiedEmails.map((email, index) => (
                    <div key={index} className="flex items-center gap-2 p-2 bg-gray-800 rounded">
                      <CheckCircle className="w-4 h-4 text-green-400" />
                      <span className="text-sm font-mono">{email}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {status.verifiedDomains.length === 0 && status.verifiedEmails.length === 0 && (
              <p className="text-sm text-gray-400 text-center py-4">
                No verified identities found
              </p>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Sandbox Information */}
      {status.inSandbox && sandboxInfo && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Info className="w-5 h-5" />
              Sandbox Mode Information
            </CardTitle>
            <CardDescription>
              Limitations and how to request production access
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Limitations */}
            <div>
              <h4 className="text-sm font-medium text-gray-300 mb-2">Current Limitations</h4>
              <ul className="space-y-1 text-sm text-gray-400">
                {sandboxInfo.limitations.map((limitation, index) => (
                  <li key={index} className="flex items-start gap-2">
                    <span className="text-yellow-400">•</span>
                    <span>{limitation}</span>
                  </li>
                ))}
              </ul>
            </div>

            {/* Exit Steps */}
            <div>
              <h4 className="text-sm font-medium text-gray-300 mb-2">How to Exit Sandbox</h4>
              <ol className="space-y-2 text-sm text-gray-400">
                {sandboxInfo.exitSteps.map((step, index) => (
                  <li key={index} className="flex items-start gap-2">
                    <span className="text-blue-400">{index + 1}.</span>
                    <span>{step}</span>
                  </li>
                ))}
              </ol>
            </div>

            {/* Required Info */}
            <div>
              <h4 className="text-sm font-medium text-gray-300 mb-2">Required for Production Access</h4>
              <ul className="space-y-1 text-sm text-gray-400">
                {sandboxInfo.requiredInfo.map((info, index) => (
                  <li key={index} className="flex items-start gap-2">
                    <span className="text-green-400">✓</span>
                    <span>{info}</span>
                  </li>
                ))}
              </ul>
            </div>

            <div className="pt-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => window.open('https://console.aws.amazon.com/ses/home#/account', '_blank')}
              >
                <ExternalLink className="w-4 h-4 mr-2" />
                Request Production Access
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Production Tips */}
      {!status.inSandbox && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <TrendingUp className="w-5 h-5" />
              Production Best Practices
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-2 text-sm text-gray-300">
              <li>• Monitor your bounce and complaint rates to maintain good sender reputation</li>
              <li>• Use configuration sets to track email metrics</li>
              <li>• Implement proper email authentication (SPF, DKIM, DMARC)</li>
              <li>• Handle bounces and complaints programmatically</li>
              <li>• Warm up your sending gradually to build reputation</li>
            </ul>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
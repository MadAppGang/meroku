import { useState, useEffect } from 'react';
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
  Clock,
  Copy
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
              <Badge variant={status.inSandbox ? 'outline' : 'default'}>
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
              <Badge variant={status.sendingEnabled ? 'default' : 'destructive'}>
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
                  status.reputationStatus === 'Healthy' ? 'default' : 
                  status.reputationStatus === 'Default' ? 'secondary' : 
                  'outline'
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
        <Card className="border-yellow-600/50">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <AlertCircle className="w-5 h-5 text-yellow-500" />
              Sandbox Mode - Request Production Access
            </CardTitle>
            <CardDescription>
              Your account is currently limited. Request production access to send emails without restrictions.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Action Buttons */}
            <div className="flex gap-2">
              <Button
                variant="default"
                onClick={() => window.open('https://console.aws.amazon.com/ses/home#/account', '_blank')}
              >
                <ExternalLink className="w-4 h-4 mr-2" />
                Request Production Access
              </Button>
              <Button
                variant="outline"
                onClick={() => window.open('https://docs.aws.amazon.com/ses/latest/dg/request-production-access.html', '_blank')}
              >
                <Info className="w-4 h-4 mr-2" />
                View Guide
              </Button>
            </div>

            {/* Current Limitations */}
            <Alert className="border-yellow-600 bg-yellow-50">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>
                <p className="font-medium mb-2">Current Sandbox Limitations:</p>
                <ul className="space-y-1 text-sm">
                  {sandboxInfo.limitations.map((limitation, index) => (
                    <li key={index}>• {limitation}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>

            {/* Production Access Request Template */}
            <div className="space-y-3">
              <h4 className="text-sm font-medium text-gray-300">Production Access Request Template</h4>
              <div className="bg-gray-800 rounded-lg p-4 space-y-3">
                <p className="text-sm text-gray-400">Use this template when submitting your production access request:</p>
                
                <div className="bg-gray-900 rounded p-3 font-mono text-xs text-gray-300 space-y-2">
                  <p><strong>Use Case Description:</strong></p>
                  <p>We are using AWS SES to send transactional emails for our {config.project || '[PROJECT NAME]'} application. Our emails include:</p>
                  <p>- User registration confirmations</p>
                  <p>- Password reset notifications</p>
                  <p>- Order confirmations and updates</p>
                  <p>- System alerts and notifications</p>
                  <p></p>
                  <p><strong>Email Volume:</strong></p>
                  <p>Expected daily volume: [SPECIFY YOUR EXPECTED VOLUME]</p>
                  <p>Peak sending rate: [SPECIFY EMAILS PER SECOND]</p>
                  <p></p>
                  <p><strong>Recipient Management:</strong></p>
                  <p>- All recipients have explicitly opted in to receive emails</p>
                  <p>- We maintain unsubscribe links in all marketing emails</p>
                  <p>- We immediately process bounce and complaint notifications</p>
                  <p>- We maintain a suppression list for opted-out addresses</p>
                  <p></p>
                  <p><strong>Content Type:</strong></p>
                  <p>Transactional emails only - no marketing or promotional content without explicit consent</p>
                  <p></p>
                  <p><strong>Compliance:</strong></p>
                  <p>We comply with CAN-SPAM Act, GDPR, and all applicable email regulations</p>
                </div>
                
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    const template = `Use Case Description:
We are using AWS SES to send transactional emails for our ${config.project || '[PROJECT NAME]'} application. Our emails include:
- User registration confirmations
- Password reset notifications
- Order confirmations and updates
- System alerts and notifications

Email Volume:
Expected daily volume: [SPECIFY YOUR EXPECTED VOLUME]
Peak sending rate: [SPECIFY EMAILS PER SECOND]

Recipient Management:
- All recipients have explicitly opted in to receive emails
- We maintain unsubscribe links in all marketing emails
- We immediately process bounce and complaint notifications
- We maintain a suppression list for opted-out addresses

Content Type:
Transactional emails only - no marketing or promotional content without explicit consent

Compliance:
We comply with CAN-SPAM Act, GDPR, and all applicable email regulations`;
                    navigator.clipboard.writeText(template);
                  }}
                >
                  <Copy className="w-3 h-3 mr-1" />
                  Copy Template
                </Button>
              </div>
            </div>

            {/* Step by Step Instructions */}
            <div className="space-y-3">
              <h4 className="text-sm font-medium text-gray-300">How to Request Production Access</h4>
              <ol className="space-y-2 text-sm text-gray-400">
                <li className="flex items-start gap-2">
                  <span className="text-blue-400 font-medium">1.</span>
                  <div>
                    <p>Go to the AWS Support Center</p>
                    <p className="text-xs mt-1">Click "Request Production Access" button above</p>
                  </div>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-blue-400 font-medium">2.</span>
                  <div>
                    <p>Create a new case with:</p>
                    <ul className="text-xs mt-1 ml-4 space-y-1">
                      <li>• Service: Service Limit Increase</li>
                      <li>• Limit Type: SES Sending Limits</li>
                      <li>• Region: {status.region}</li>
                    </ul>
                  </div>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-blue-400 font-medium">3.</span>
                  <div>
                    <p>Fill in the request details:</p>
                    <ul className="text-xs mt-1 ml-4 space-y-1">
                      <li>• Use the template above</li>
                      <li>• Be specific about your use case</li>
                      <li>• Include your website URL</li>
                    </ul>
                  </div>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-blue-400 font-medium">4.</span>
                  <span>Submit and wait 24-48 hours for approval</span>
                </li>
              </ol>
            </div>

            {/* Best Practices */}
            <Alert>
              <Info className="h-4 w-4" />
              <AlertDescription>
                <p className="font-medium mb-1">Tips for Approval:</p>
                <ul className="text-xs space-y-1">
                  <li>• Clearly explain your business use case</li>
                  <li>• Demonstrate you have bounce/complaint handling</li>
                  <li>• Show you maintain opt-in/opt-out mechanisms</li>
                  <li>• Include your privacy policy URL</li>
                </ul>
              </AlertDescription>
            </Alert>
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
import React, { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Textarea } from './ui/textarea';
import { Button } from './ui/button';
import { Alert, AlertDescription } from './ui/alert';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { infrastructureApi } from '../api/infrastructure';
import { 
  Send,
  Mail,
  AlertCircle,
  CheckCircle,
  Info,
  Copy,
  FileText,
  Loader2
} from 'lucide-react';

interface SESSendTestEmailProps {
  config: YamlInfrastructureConfig;
}

export function SESSendTestEmail({ config }: SESSendTestEmailProps) {
  const [to, setTo] = useState('');
  const [subject, setSubject] = useState('Test Email from SES');
  const [body, setBody] = useState(`This is a test email sent from AWS SES.

Environment: ${config.env}
Project: ${config.project}
Region: ${config.region}

This email confirms that your SES configuration is working correctly.`);
  
  const [sending, setSending] = useState(false);
  const [response, setResponse] = useState<{ success: boolean; message: string; messageId?: string } | null>(null);
  
  // Determine the from address
  const sesConfig = config.ses || { enabled: false };
  const isDomainEnabled = config.domain?.enabled;
  const mainDomain = config.domain?.domain_name;
  const isProd = config.env === 'prod' || config.env === 'production';
  const defaultDomain = mainDomain ? 
    (isProd ? `mail.${mainDomain}` : `mail.${config.env}.${mainDomain}`) : '';
  const actualDomain = sesConfig.domain_name || defaultDomain || 'example.com';
  const fromAddress = `noreply@${actualDomain}`;

  const handleSendEmail = async () => {
    if (!to || !subject || !body) {
      setResponse({
        success: false,
        message: 'Please fill in all fields'
      });
      return;
    }

    setSending(true);
    setResponse(null);

    try {
      const result = await infrastructureApi.sendTestEmail(to, subject, body);
      setResponse({
        success: true,
        message: 'Test email sent successfully!',
        messageId: result.messageId
      });
    } catch (error) {
      setResponse({
        success: false,
        message: error instanceof Error ? error.message : 'Failed to send test email'
      });
    } finally {
      setSending(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  // Email templates
  const templates = [
    {
      name: 'Simple Test',
      subject: 'Test Email from SES',
      body: `This is a test email sent from AWS SES.

Environment: ${config.env}
Project: ${config.project}
Region: ${config.region}

This email confirms that your SES configuration is working correctly.`
    },
    {
      name: 'HTML Test',
      subject: 'HTML Test Email',
      body: `<html>
<body style="font-family: Arial, sans-serif; padding: 20px;">
  <h2>Test Email from AWS SES</h2>
  <p>This is a <strong>test email</strong> with HTML content.</p>
  <ul>
    <li>Environment: <code>${config.env}</code></li>
    <li>Project: <code>${config.project}</code></li>
    <li>Region: <code>${config.region}</code></li>
  </ul>
  <p style="color: #666; font-size: 12px;">
    Sent from AWS SES via ${config.project} infrastructure
  </p>
</body>
</html>`
    },
    {
      name: 'Welcome Email',
      subject: 'Welcome to ${project}!',
      body: `Hello,

Welcome to ${config.project}! We're excited to have you on board.

This is a test of our transactional email system powered by AWS SES.

If you're receiving this email, it means our email infrastructure is working correctly.

Best regards,
The ${config.project} Team

--
This is an automated message from ${actualDomain}`
    }
  ];

  return (
    <div className="space-y-4">
      {/* Info Alert */}
      <Alert>
        <Info className="h-4 w-4" />
        <AlertDescription>
          Test emails will be sent from <code className="text-blue-400">{fromAddress}</code>
          {sesConfig.enabled ? '' : ' (SES must be enabled and deployed first)'}
        </AlertDescription>
      </Alert>

      {/* Email Form */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Mail className="w-5 h-5" />
            Compose Test Email
          </CardTitle>
          <CardDescription>
            Send a test email to verify your SES configuration
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="to-email">To Email Address</Label>
            <Input
              id="to-email"
              type="email"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              placeholder="recipient@example.com"
              disabled={sending}
            />
            <p className="text-xs text-gray-500">
              In sandbox mode, this must be a verified email address
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="subject">Subject</Label>
            <Input
              id="subject"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              placeholder="Test Email Subject"
              disabled={sending}
            />
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="body">Email Body</Label>
              <span className="text-xs text-gray-500">Plain text or HTML</span>
            </div>
            <Textarea
              id="body"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder="Email content..."
              className="min-h-[200px] font-mono text-sm"
              disabled={sending}
            />
          </div>

          <div className="flex gap-2">
            <Button 
              onClick={handleSendEmail} 
              disabled={sending || !to || !subject || !body}
              className="flex-1"
            >
              {sending ? (
                <>
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  Sending...
                </>
              ) : (
                <>
                  <Send className="w-4 h-4 mr-2" />
                  Send Test Email
                </>
              )}
            </Button>
          </div>

          {response && (
            <Alert className={response.success ? 'border-green-600' : 'border-red-600'}>
              {response.success ? (
                <CheckCircle className="h-4 w-4 text-green-600" />
              ) : (
                <AlertCircle className="h-4 w-4 text-red-600" />
              )}
              <AlertDescription>
                <div className="space-y-1">
                  <p>{response.message}</p>
                  {response.messageId && (
                    <p className="text-xs font-mono">
                      Message ID: {response.messageId}
                    </p>
                  )}
                </div>
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      {/* Email Templates */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <FileText className="w-5 h-5" />
            Email Templates
          </CardTitle>
          <CardDescription>
            Quick templates for testing different email scenarios
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {templates.map((template, index) => (
            <div key={index} className="p-3 bg-gray-800 rounded-lg">
              <div className="flex items-center justify-between mb-2">
                <h4 className="text-sm font-medium">{template.name}</h4>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setSubject(template.subject);
                    setBody(template.body);
                  }}
                >
                  Use Template
                </Button>
              </div>
              <p className="text-xs text-gray-400 mb-1">Subject: {template.subject}</p>
              <div className="bg-gray-900 rounded p-2 max-h-20 overflow-hidden">
                <pre className="text-xs text-gray-500 whitespace-pre-wrap">
                  {template.body.substring(0, 150)}...
                </pre>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>

      {/* Testing Tips */}
      <Card>
        <CardHeader>
          <CardTitle>Testing Tips</CardTitle>
          <CardDescription>
            Best practices for testing email delivery
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ul className="space-y-2 text-sm text-gray-300">
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>In sandbox mode, you can only send to verified email addresses</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Check your spam folder if the email doesn't arrive</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Test both plain text and HTML formats</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Verify SPF, DKIM, and DMARC are properly configured</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Monitor CloudWatch for bounce and complaint metrics</span>
            </li>
          </ul>
        </CardContent>
      </Card>

      {/* Email Headers Preview */}
      <Card>
        <CardHeader>
          <CardTitle>Email Headers Preview</CardTitle>
          <CardDescription>
            Headers that will be included in your test email
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300 space-y-1">
            <div className="flex items-center justify-between">
              <span>
                <span className="text-gray-500">From:</span> {fromAddress}
              </span>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => copyToClipboard(fromAddress)}
              >
                <Copy className="w-3 h-3" />
              </Button>
            </div>
            <div>
              <span className="text-gray-500">To:</span> {to || 'recipient@example.com'}
            </div>
            <div>
              <span className="text-gray-500">Subject:</span> {subject || 'Test Email Subject'}
            </div>
            <div>
              <span className="text-gray-500">X-SES-Configuration-Set:</span> {config.project}-{config.env}
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
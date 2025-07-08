import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Label } from './ui/label';
import { 
  Globe, 
  Shield, 
  Activity, 
  FileText, 
  Route,
  Zap,
  Clock,
  Link
} from 'lucide-react';
import type { YamlInfrastructureConfig } from '../types/yamlConfig';

interface APIGatewayPropertiesProps {
  config: YamlInfrastructureConfig;
}

export function APIGatewayProperties({ config }: APIGatewayPropertiesProps) {
  const apiName = `${config.project}-${config.env}`;
  const logGroup = `/aws/api_gateway/${apiName}`;
  
  return (
    <div className="space-y-4">
      {/* Basic Info */}
      <Card className="bg-gray-800 border-gray-700">
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
            <Globe className="w-4 h-4" />
            Basic Information
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div>
            <Label className="text-xs text-gray-400">API Name</Label>
            <div className="text-sm text-white font-mono">{apiName}</div>
          </div>
          <div>
            <Label className="text-xs text-gray-400">Protocol</Label>
            <div className="text-sm text-white">HTTP API</div>
          </div>
          <div>
            <Label className="text-xs text-gray-400">Stage</Label>
            <div className="text-sm text-white">{config.env}</div>
          </div>
          <div>
            <Label className="text-xs text-gray-400">Endpoint Type</Label>
            <div className="text-sm text-white">Regional</div>
          </div>
        </CardContent>
      </Card>

      {/* Domain Mapping */}
      <Card className="bg-gray-800 border-gray-700">
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
            <Link className="w-4 h-4" />
            Domain Mapping
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div>
            <Label className="text-xs text-gray-400">Custom Domain</Label>
            <div className="text-sm text-white font-mono">
              {config.api_domain || `api.${config.project}.com`}
            </div>
          </div>
          <div>
            <Label className="text-xs text-gray-400">Certificate</Label>
            <div className="text-sm text-white flex items-center gap-2">
              <Shield className="w-3 h-3 text-green-400" />
              TLS 1.2 with ACM certificate
            </div>
          </div>
          <div>
            <Label className="text-xs text-gray-400">Route 53</Label>
            <div className="text-sm text-white">A record → API Gateway</div>
          </div>
        </CardContent>
      </Card>

      {/* Stage Configuration */}
      <Card className="bg-gray-800 border-gray-700">
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
            <Zap className="w-4 h-4" />
            Stage Configuration
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div>
            <Label className="text-xs text-gray-400">Auto-Deploy</Label>
            <div className="text-sm text-white flex items-center gap-2">
              <Activity className="w-3 h-3 text-green-400" />
              Enabled
            </div>
          </div>
          <div>
            <Label className="text-xs text-gray-400">Throttling</Label>
            <div className="text-sm text-white space-y-1">
              <div className="flex items-center gap-2">
                <span className="text-gray-400">Burst:</span>
                <span className="font-mono">5,000 requests</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-gray-400">Rate:</span>
                <span className="font-mono">10,000 requests/second</span>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Logging */}
      <Card className="bg-gray-800 border-gray-700">
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
            <FileText className="w-4 h-4" />
            Logging
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div>
            <Label className="text-xs text-gray-400">CloudWatch Log Group</Label>
            <div className="text-sm text-white font-mono break-all">{logGroup}</div>
          </div>
          <div>
            <Label className="text-xs text-gray-400">Retention</Label>
            <div className="text-sm text-white flex items-center gap-2">
              <Clock className="w-3 h-3 text-gray-400" />
              30 days
            </div>
          </div>
          <div>
            <Label className="text-xs text-gray-400">Log Format</Label>
            <div className="text-sm text-white">JSON with request details</div>
          </div>
        </CardContent>
      </Card>

      {/* Routes */}
      <Card className="bg-gray-800 border-gray-700">
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
            <Route className="w-4 h-4" />
            Routes
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="space-y-2">
            <div className="text-sm">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-gray-400">Backend Route:</span>
              </div>
              <div className="font-mono text-xs bg-gray-900 p-2 rounded">
                ANY /{'{proxy+}'} → Backend service
              </div>
            </div>
            
            {config.services && config.services.length > 0 && (
              <div className="text-sm">
                <div className="flex items-center gap-2 mb-1">
                  <span className="text-gray-400">Service Routes:</span>
                </div>
                <div className="space-y-1">
                  {config.services.map((service) => (
                    <div key={service.name} className="font-mono text-xs bg-gray-900 p-2 rounded">
                      ANY /{service.name}/{'{proxy+}'} → {service.name} service
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Network, ArrowRight } from 'lucide-react';
import type { YamlInfrastructureConfig } from '../types/yamlConfig';

interface ALBRoutingRulesProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: YamlInfrastructureConfig) => void;
}

export const ALBRoutingRules: React.FC<ALBRoutingRulesProps> = ({ config }) => {
  // Default routing rules for ALB
  const defaultRules = [
    {
      priority: 1,
      condition: 'HTTPS:443',
      action: 'Forward to Backend Target Group',
      description: 'Default Action'
    },
    {
      priority: 2,
      condition: 'HTTP:80',
      action: 'Redirect to HTTPS',
      description: 'Redirect to port 443 (Permanent)'
    }
  ];

  // Add service-specific rules if services are configured
  const serviceRules = config.services?.map((service, index) => ({
    priority: index + 3,
    condition: `Path: /${service.name}/*`,
    action: `Forward to ${service.name} Target Group`,
    description: `Route to ${service.name} service`
  })) || [];

  const allRules = [...defaultRules, ...serviceRules];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Network className="w-5 h-5" />
          Listener Rules
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          <div className="text-sm text-gray-400 mb-4">
            ALB routing rules are configured automatically based on your services.
          </div>
          
          {allRules.map((rule) => (
            <div key={rule.priority} className="bg-gray-800 rounded-lg p-3">
              <div className="flex items-center justify-between">
                <div className="flex-1">
                  <div className="flex items-center gap-3 mb-2">
                    <span className="text-xs bg-blue-500/20 text-blue-400 px-2 py-1 rounded">
                      Priority {rule.priority}
                    </span>
                    <span className="text-xs text-gray-500">
                      {rule.description}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 text-sm">
                    <span className="text-gray-400">{rule.condition}</span>
                    <ArrowRight className="w-4 h-4 text-gray-600" />
                    <span className="text-gray-300">{rule.action}</span>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
};
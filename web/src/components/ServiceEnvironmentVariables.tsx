import React, { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { type AccountInfo } from '../api/infrastructure';
import { ComponentNode } from '../types';
import { Lock, Info, AlertCircle, FileText, HardDrive, Plus, Trash2, Edit2, Check, X, Settings } from 'lucide-react';
import { Alert, AlertDescription } from './ui/alert';
import { Input } from './ui/input';
import { Button } from './ui/button';
import { Label } from './ui/label';

interface ServiceEnvironmentVariablesProps {
  config: YamlInfrastructureConfig;
  accountInfo?: AccountInfo;
  node: ComponentNode;
  onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function ServiceEnvironmentVariables({ config, accountInfo, node, onConfigChange }: ServiceEnvironmentVariablesProps) {
  // Extract service name from node id
  const serviceName = node.id.replace('service-', '');
  
  // Find the service configuration
  const serviceConfig = config.services?.find(service => service.name === serviceName);

  // State for custom environment variables
  const [customVars, setCustomVars] = useState<Record<string, string>>(serviceConfig?.env_vars || {});
  const [newVarName, setNewVarName] = useState('');
  const [newVarValue, setNewVarValue] = useState('');
  const [editingVar, setEditingVar] = useState<string | null>(null);
  const [editingVarValue, setEditingVarValue] = useState('');

  if (!serviceConfig) {
    return (
      <Alert className="border-red-600">
        <AlertCircle className="h-4 w-4 text-red-600" />
        <AlertDescription>
          Service "{serviceName}" not found in configuration.
        </AlertDescription>
      </Alert>
    );
  }

  // Preconfigured environment variables
  const preconfiguredEnvVars = [
    // PostgreSQL variables - always shown but marked if not enabled
    { 
      name: 'PG_DATABASE_HOST', 
      value: config.postgres?.enabled ? `${config.project}-${config.env}-rds.${config.region}.rds.amazonaws.com` : 'Not available (PostgreSQL disabled)', 
      description: 'PostgreSQL endpoint',
      enabled: config.postgres?.enabled
    },
    { 
      name: 'PG_DATABASE_USERNAME', 
      value: config.postgres?.enabled ? (config.postgres?.username || 'postgres') : 'Not available (PostgreSQL disabled)', 
      description: 'PostgreSQL username',
      enabled: config.postgres?.enabled
    },
    { 
      name: 'PG_DATABASE_NAME', 
      value: config.postgres?.enabled ? (config.postgres?.dbname || 'database') : 'Not available (PostgreSQL disabled)', 
      description: 'PostgreSQL database name',
      enabled: config.postgres?.enabled
    },
    { 
      name: 'AWS_REGION', 
      value: config.region, 
      description: 'Current AWS region',
      enabled: true
    },
    { 
      name: 'URL', 
      value: config.workload?.backend_alb_domain_name || `api.${config.domain?.domain_name || ''}`, 
      description: 'API domain URL',
      enabled: true
    },
    // SQS variables - always shown but marked if not enabled
    {
      name: 'SQS_QUEUE_URL',
      value: config.sqs?.enabled ? `https://sqs.${config.region}.amazonaws.com/${accountInfo?.accountId || config.ecr_account_id || '<ACCOUNT_ID>'}/${config.project}-${config.env}-${config.sqs.name || 'queue'}` : 'Not available (SQS disabled)',
      description: 'SQS Queue URL',
      enabled: config.sqs?.enabled
    },
    {
      name: 'AWS_QUEUE_URL',
      value: config.sqs?.enabled ? `https://sqs.${config.region}.amazonaws.com/${accountInfo?.accountId || config.ecr_account_id || '<ACCOUNT_ID>'}/${config.project}-${config.env}-${config.sqs.name || 'queue'}` : 'Not available (SQS disabled)',
      description: 'SQS Queue URL (duplicate)',
      enabled: config.sqs?.enabled
    },
    // X-Ray variable - always shown but marked if not enabled
    {
      name: 'ADOT_COLLECTOR_URL',
      value: serviceConfig.xray_enabled ? 'localhost:2000' : 'Not available (X-Ray disabled)',
      description: 'X-Ray collector URL',
      enabled: serviceConfig.xray_enabled
    }
  ];

  const parameterStorePath = `/${config.env}/${config.project}/${serviceName}`;

  // Handler functions for custom variables
  const handleAddCustomVar = () => {
    if (newVarName && newVarValue) {
      const updatedVars = { ...customVars, [newVarName]: newVarValue };
      setCustomVars(updatedVars);
      setNewVarName('');
      setNewVarValue('');
      
      // Update config if handler is provided
      if (onConfigChange && config.services) {
        const updatedServices = config.services.map(service => 
          service.name === serviceName 
            ? { ...service, env_vars: updatedVars }
            : service
        );
        onConfigChange({ services: updatedServices });
      }
    }
  };

  const handleDeleteCustomVar = (name: string) => {
    const updatedVars = { ...customVars };
    delete updatedVars[name];
    setCustomVars(updatedVars);
    
    // Update config if handler is provided
    if (onConfigChange && config.services) {
      const updatedServices = config.services.map(service => 
        service.name === serviceName 
          ? { ...service, env_vars: updatedVars }
          : service
      );
      onConfigChange({ services: updatedServices });
    }
  };

  const handleEditCustomVar = (name: string, newValue: string) => {
    const updatedVars = { ...customVars, [name]: newValue };
    setCustomVars(updatedVars);
    setEditingVar(null);
    
    // Update config if handler is provided
    if (onConfigChange && config.services) {
      const updatedServices = config.services.map(service => 
        service.name === serviceName 
          ? { ...service, env_vars: updatedVars }
          : service
      );
      onConfigChange({ services: updatedServices });
    }
  };

  return (
    <div className="space-y-4">
      {/* Preconfigured Environment Variables */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Lock className="w-4 h-4" />
            Preconfigured Environment Variables
          </CardTitle>
          <CardDescription>Variables automatically set for all services</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {preconfiguredEnvVars.map((envVar) => (
              <div key={envVar.name} className={`p-2 rounded-lg ${envVar.enabled ? 'bg-gray-800' : 'bg-gray-800/50'}`}>
                <code className={`text-xs font-mono ${envVar.enabled ? 'text-blue-400' : 'text-gray-500'}`}>{envVar.name}</code>
                <div className={`text-xs font-mono break-all mt-1 ${envVar.enabled ? 'text-gray-300' : 'text-gray-500'}`}>{envVar.value}</div>
                <p className="text-xs text-gray-500 mt-1">{envVar.description}</p>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Custom Environment Variables */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Settings className="w-4 h-4" />
            Custom Environment Variables
          </CardTitle>
          <CardDescription>Add your own environment variables for {serviceName}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {Object.entries(customVars).map(([name, value]) => (
              <div key={name} className="p-2 bg-gray-800 rounded">
                <div className="flex items-center justify-between">
                  <code className="text-xs font-mono text-green-400">{name}</code>
                  <div className="flex items-center gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        setEditingVar(name);
                        setEditingVarValue(value);
                      }}
                      className="h-5 w-5 p-0"
                    >
                      <Edit2 className="w-3 h-3" />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => handleDeleteCustomVar(name)}
                      className="h-5 w-5 p-0 text-red-400 hover:text-red-300"
                    >
                      <Trash2 className="w-3 h-3" />
                    </Button>
                  </div>
                </div>
                {editingVar === name ? (
                  <div className="mt-1">
                    <Input
                      type="text"
                      value={editingVarValue}
                      onChange={(e) => setEditingVarValue(e.target.value)}
                      className="w-full h-6 text-xs"
                    />
                    <div className="flex items-center gap-1 mt-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleEditCustomVar(name, editingVarValue)}
                        className="h-5 w-5 p-0"
                      >
                        <Check className="w-3 h-3" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setEditingVar(null)}
                        className="h-5 w-5 p-0"
                      >
                        <X className="w-3 h-3" />
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="text-xs text-gray-300 font-mono mt-1">{value}</div>
                )}
              </div>
            ))}
            
            {/* Add new variable form */}
            <div className="border-t border-gray-700 pt-3">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <Label htmlFor="new-var-name" className="text-xs text-gray-400">Variable Name</Label>
                  <Input
                    id="new-var-name"
                    type="text"
                    placeholder="VARIABLE_NAME"
                    value={newVarName}
                    onChange={(e) => setNewVarName(e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, '_'))}
                    className="mt-1"
                  />
                </div>
                <div>
                  <Label htmlFor="new-var-value" className="text-xs text-gray-400">Value</Label>
                  <Input
                    id="new-var-value"
                    type="text"
                    placeholder="value"
                    value={newVarValue}
                    onChange={(e) => setNewVarValue(e.target.value)}
                    className="mt-1"
                  />
                </div>
              </div>
              <Button
                onClick={handleAddCustomVar}
                disabled={!newVarName || !newVarValue}
                className="mt-2 w-full"
                size="sm"
              >
                <Plus className="w-4 h-4 mr-2" />
                Add Variable
              </Button>
            </div>
          </div>

          <Alert className="mt-4">
            <Info className="h-4 w-4" />
            <AlertDescription className="text-xs">
              These variables are defined in the YAML configuration. For secrets, use Parameter Store at <code>{parameterStorePath}/*</code>
            </AlertDescription>
          </Alert>
        </CardContent>
      </Card>

    </div>
  );
}
import { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { type AccountInfo } from '../api/infrastructure';
import { Input } from './ui/input';
import { Button } from './ui/button';
import { Plus, Trash2, Edit2, Check, X, Lock, Settings } from 'lucide-react';
import { Label } from './ui/label';

interface BackendEnvironmentVariablesProps {
  config: YamlInfrastructureConfig;
  accountInfo?: AccountInfo;
}

export function BackendEnvironmentVariables({ config, accountInfo }: BackendEnvironmentVariablesProps) {
  const [editingPort, setEditingPort] = useState(false);
  const [portValue, setPortValue] = useState(config.workload?.backend_image_port?.toString() || '8080');
  const [customVars, setCustomVars] = useState(config.workload?.backend_env_variables || []);
  const [newVarName, setNewVarName] = useState('');
  const [newVarValue, setNewVarValue] = useState('');
  const [editingVar, setEditingVar] = useState<string | null>(null);
  const [editingVarValue, setEditingVarValue] = useState('');

  // Automatic environment variables (infrastructure-derived)
  const automaticEnvVars = [
    { name: 'PG_DATABASE_HOST', value: `${config.project}-${config.env}-rds.${config.region}.rds.amazonaws.com`, description: 'RDS Instance Endpoint' },
    { name: 'PG_DATABASE_USERNAME', value: config.postgres?.username || 'postgres', description: 'Database Username' },
    { name: 'PG_DATABASE_NAME', value: config.postgres?.dbname || 'database', description: 'Database Name' },
    { name: 'AWS_S3_BUCKET', value: `${config.project}-backend-${config.env}${config.workload?.bucket_postfix ? `-${config.workload.bucket_postfix}` : ''}`, description: 'S3 Bucket for Backend' },
    { name: 'AWS_REGION', value: config.region, description: 'AWS Region' },
    { name: 'URL', value: config.workload?.backend_alb_domain_name || `api.${config.domain?.domain_name || ''}`, description: 'API Domain URL' },
    { name: 'SQS_QUEUE_URL', value: `https://sqs.${config.region}.amazonaws.com/${accountInfo?.accountId || config.ecr_account_id || '<ACCOUNT_ID>'}/${config.project}-${config.env}-${config.sqs?.name || 'queue'}`, description: 'SQS Queue URL' },
    { name: 'AWS_QUEUE_URL', value: `https://sqs.${config.region}.amazonaws.com/${accountInfo?.accountId || config.ecr_account_id || '<ACCOUNT_ID>'}/${config.project}-${config.env}-${config.sqs?.name || 'queue'}`, description: 'SQS Queue URL (Alias)' },
  ];

  // Configurable environment variables
  const configurableEnvVars = [
    { name: 'PORT', value: portValue, description: 'Application Port', editable: true },
  ];


  const handleAddCustomVar = () => {
    if (newVarName && newVarValue) {
      setCustomVars([...customVars, { name: newVarName, value: newVarValue }]);
      setNewVarName('');
      setNewVarValue('');
    }
  };

  const handleDeleteCustomVar = (name: string) => {
    setCustomVars(customVars.filter(v => v.name !== name));
  };

  const handleEditCustomVar = (name: string, newValue: string) => {
    setCustomVars(customVars.map(v => v.name === name ? { ...v, value: newValue } : v));
    setEditingVar(null);
  };

  return (
    <div className="space-y-4">
      {/* Automatic Environment Variables */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Lock className="w-4 h-4" />
            Automatic Environment Variables
          </CardTitle>
          <CardDescription>Variables automatically derived from infrastructure configuration</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {automaticEnvVars.map((envVar) => (
              <div key={envVar.name} className="p-2 bg-gray-800 rounded-lg">
                <code className="text-xs font-mono text-blue-400">{envVar.name}</code>
                <div className="text-xs text-gray-300 font-mono break-all mt-1">{envVar.value}</div>
                <p className="text-xs text-gray-500 mt-1">{envVar.description}</p>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Configurable Environment Variables */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Settings className="w-4 h-4" />
            Configurable Environment Variables
          </CardTitle>
          <CardDescription>Variables you can modify</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {configurableEnvVars.map((envVar) => (
              <div key={envVar.name} className="p-2 bg-gray-800 rounded-lg">
                <div className="flex items-center justify-between">
                  <code className="text-xs font-mono text-orange-400">{envVar.name}</code>
                  {envVar.editable && (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setEditingPort(true)}
                      className="h-5 w-5 p-0"
                    >
                      <Edit2 className="w-3 h-3" />
                    </Button>
                  )}
                </div>
                {editingPort && envVar.name === 'PORT' ? (
                  <div className="mt-1">
                    <Input
                      type="text"
                      value={portValue}
                      onChange={(e: React.ChangeEvent<HTMLInputElement>) => setPortValue(e.target.value)}
                      className="w-full h-6 text-xs"
                    />
                    <div className="flex items-center gap-1 mt-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setEditingPort(false)}
                        className="h-5 w-5 p-0"
                      >
                        <Check className="w-3 h-3" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          setPortValue(config.workload?.backend_image_port?.toString() || '8080');
                          setEditingPort(false);
                        }}
                        className="h-5 w-5 p-0"
                      >
                        <X className="w-3 h-3" />
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="text-xs text-gray-300 font-mono mt-1">{envVar.value}</div>
                )}
                <p className="text-xs text-gray-500 mt-1">{envVar.description}</p>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Custom Environment Variables */}
      <Card>
        <CardHeader>
          <CardTitle>Custom Environment Variables</CardTitle>
          <CardDescription>Add your own environment variables</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {customVars.map((envVar, index) => (
              <div key={index} className="p-2 bg-gray-800 rounded">
                <div className="flex items-center justify-between">
                  <code className="text-xs font-mono text-green-400">{envVar.name}</code>
                  <div className="flex items-center gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        setEditingVar(envVar.name);
                        setEditingVarValue(envVar.value);
                      }}
                      className="h-5 w-5 p-0"
                    >
                      <Edit2 className="w-3 h-3" />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => handleDeleteCustomVar(envVar.name)}
                      className="h-5 w-5 p-0 text-red-400 hover:text-red-300"
                    >
                      <Trash2 className="w-3 h-3" />
                    </Button>
                  </div>
                </div>
                {editingVar === envVar.name ? (
                  <div className="mt-1">
                    <Input
                      type="text"
                      value={editingVarValue}
                      onChange={(e: React.ChangeEvent<HTMLInputElement>) => setEditingVarValue(e.target.value)}
                      className="w-full h-6 text-xs"
                    />
                    <div className="flex items-center gap-1 mt-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleEditCustomVar(envVar.name, editingVarValue)}
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
                  <div className="text-xs text-gray-300 font-mono mt-1">{envVar.value}</div>
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
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewVarName(e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, '_'))}
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
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewVarValue(e.target.value)}
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
        </CardContent>
      </Card>
    </div>
  );
}
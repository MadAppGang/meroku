import { useState } from 'react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { RadioGroup, RadioGroupItem } from './ui/radio-group';
import { Info } from 'lucide-react';
import { AWS_REGIONS } from '../types/config';
import { type AccountInfo } from '../api/infrastructure';

interface ECRNodePropertiesProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
  accountInfo?: AccountInfo;
}

export function ECRNodeProperties({ config, onConfigChange, accountInfo }: ECRNodePropertiesProps) {
  const [ecrMode, setEcrMode] = useState<'create' | 'cross-account'>(
    config.ecr_account_id ? 'cross-account' : 'create'
  );


  const handleModeChange = (value: string) => {
    const mode = value as 'create' | 'cross-account';
    setEcrMode(mode);
    
    if (mode === 'create') {
      // Clear cross-account settings
      const newConfig = { ...config };
      delete newConfig.ecr_account_id;
      delete newConfig.ecr_account_region;
      onConfigChange(newConfig);
    }
  };

  const handleAccountIdChange = (value: string) => {
    onConfigChange({
      ...config,
      ecr_account_id: value || undefined
    });
  };

  const handleRegionChange = (value: string) => {
    onConfigChange({
      ...config,
      ecr_account_region: value || undefined
    });
  };


  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>ECR Configuration</CardTitle>
          <CardDescription>
            Configure container registry for your application
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <RadioGroup value={ecrMode} onValueChange={handleModeChange}>
            <div className="flex items-center space-x-2 p-3 rounded-lg border border-gray-700 hover:bg-gray-800">
              <RadioGroupItem value="create" id="create-ecr" />
              <Label htmlFor="create-ecr" className="flex-1 cursor-pointer">
                <div className="font-medium">Create ECR Repository</div>
                <div className="text-xs text-gray-400">
                  Create a new ECR repository in this AWS account
                </div>
              </Label>
            </div>
            <div className="flex items-center space-x-2 p-3 rounded-lg border border-gray-700 hover:bg-gray-800">
              <RadioGroupItem value="cross-account" id="cross-account-ecr" />
              <Label htmlFor="cross-account-ecr" className="flex-1 cursor-pointer">
                <div className="font-medium">Use Cross-Account ECR</div>
                <div className="text-xs text-gray-400">
                  Use an ECR repository from another AWS account
                </div>
              </Label>
            </div>
          </RadioGroup>

          {ecrMode === 'create' && (
            <div className="mt-4 space-y-3">
              <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
                <div className="flex items-start gap-2">
                  <Info className="w-4 h-4 text-blue-400 mt-0.5" />
                  <div className="space-y-1">
                    <div className="text-xs text-gray-300">
                      ECR repositories will be created automatically for each service in your infrastructure
                    </div>
                    <div className="text-xs text-gray-400">
                      Note: Repositories are only created in the <code className="text-blue-400">dev</code> environment
                    </div>
                  </div>
                </div>
              </div>
              
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Label className="text-xs text-gray-400">Repository Naming</Label>
                  <div className="text-sm font-medium">{config.project}_[type]_[name]</div>
                </div>
                <div>
                  <Label className="text-xs text-gray-400">Region</Label>
                  <div className="text-sm font-medium">{config.region}</div>
                </div>
              </div>
              
              {accountInfo && (
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <Label className="text-xs text-gray-400">AWS Account ID</Label>
                    <div className="text-sm font-mono">{accountInfo.accountId}</div>
                  </div>
                  <div>
                    <Label className="text-xs text-gray-400">AWS Profile</Label>
                    <div className="text-sm font-medium">{accountInfo.profile}</div>
                  </div>
                </div>
              )}
              
              <div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-3">
                <h4 className="text-xs font-medium text-yellow-400 mb-2">Repository Creation Rules</h4>
                <ul className="text-xs text-gray-300 space-y-1">
                  <li>• Repositories are created only in <code className="text-yellow-400">dev</code> environment</li>
                  <li>• Repository names follow pattern: <code className="text-yellow-400">{config.project}_[type]_[name]</code></li>
                  <li>• Types: <code>backend</code>, <code>service</code>, <code>task</code></li>
                  <li>• Names must be unique within the AWS account</li>
                </ul>
              </div>
              
            </div>
          )}

          {ecrMode === 'cross-account' && (
            <div className="mt-4 space-y-4">
              <div>
                <Label htmlFor="ecr-account-id">ECR Account ID</Label>
                <Input
                  id="ecr-account-id"
                  value={config.ecr_account_id || ''}
                  onChange={(e) => handleAccountIdChange(e.target.value)}
                  placeholder="123456789012"
                  className="mt-1 bg-gray-800 border-gray-600 text-white font-mono"
                />
                <p className="text-xs text-gray-400 mt-1">
                  The AWS account ID where the ECR repository exists
                </p>
              </div>

              <div>
                <Label htmlFor="ecr-region">ECR Region</Label>
                <select
                  id="ecr-region"
                  value={config.ecr_account_region || config.region}
                  onChange={(e) => handleRegionChange(e.target.value)}
                  className="mt-1 w-full bg-gray-800 border-gray-600 text-white rounded-md px-3 py-2 text-sm"
                >
                  {AWS_REGIONS.map((region) => (
                    <option key={region.value} value={region.value}>
                      {region.label} ({region.value})
                    </option>
                  ))}
                </select>
                <p className="text-xs text-gray-400 mt-1">
                  The region where the ECR repository is located
                </p>
              </div>

              {config.ecr_account_id && (
                <div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-yellow-400 mb-2">Required Permissions</h4>
                  <p className="text-xs text-gray-300 mb-2">
                    The cross-account ECR repository must have a policy that allows this account to pull images:
                  </p>
                  <pre className="text-xs text-gray-400 overflow-x-auto bg-gray-800 p-2 rounded">
{`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {
      "AWS": "arn:aws:iam::${config.ecr_account_id}:root"
    },
    "Action": [
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:BatchCheckLayerAvailability"
    ]
  }]
}`}</pre>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
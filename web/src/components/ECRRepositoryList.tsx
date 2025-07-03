import React, { useState } from 'react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Button } from './ui/button';
import { Label } from './ui/label';
import { Loader2, Copy, Check } from 'lucide-react';
import { type AccountInfo } from '../api/infrastructure';

interface ECRRepositoryListProps {
  config: YamlInfrastructureConfig;
  accountInfo?: AccountInfo;
}

export function ECRRepositoryList({ config, accountInfo }: ECRRepositoryListProps) {
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);

  const handleCopyUrl = (url: string) => {
    navigator.clipboard.writeText(url);
    setCopiedUrl(url);
    setTimeout(() => setCopiedUrl(null), 2000);
  };

  const isUsingCrossAccount = config.ecr_account_id ? true : false;
  const accountId = isUsingCrossAccount ? config.ecr_account_id : accountInfo?.accountId;
  const region = isUsingCrossAccount && config.ecr_account_region ? config.ecr_account_region : config.region;

  if (!accountInfo && !isUsingCrossAccount) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="w-6 h-6 animate-spin text-gray-400" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label className="text-sm font-medium">ECR Repositories</Label>
        <p className="text-xs text-gray-400">
          {isUsingCrossAccount 
            ? `Using cross-account ECR in ${accountId}`
            : 'Repositories that will be created in your AWS account'
          }
        </p>
      </div>

      {accountId && (
        <div className="space-y-2">
          <div className="bg-gray-800 rounded-lg p-3 space-y-2">
            <div className="text-xs font-medium text-gray-300 mb-2">Main Backend Service</div>
            <div className="space-y-1">
              <div className="text-blue-400 font-mono text-xs">{config.project}_backend</div>
              <div className="flex items-center gap-2 group">
                <div className="text-gray-500 font-mono text-xs overflow-x-auto whitespace-nowrap scrollbar-thin scrollbar-thumb-gray-600 scrollbar-track-transparent">
                  {accountId}.dkr.ecr.{region}.amazonaws.com/{config.project}_backend
                </div>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
                  onClick={() => handleCopyUrl(`${accountId}.dkr.ecr.${region}.amazonaws.com/${config.project}_backend`)}
                >
                  {copiedUrl === `${accountId}.dkr.ecr.${region}.amazonaws.com/${config.project}_backend` ? (
                    <Check className="h-3 w-3 text-green-400" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>
          
          {config.services && config.services.length > 0 && (
            <div className="bg-gray-800 rounded-lg p-3 space-y-2">
              <div className="text-xs font-medium text-gray-300 mb-2">Additional Services</div>
              {config.services.map((service, index) => (
                <div key={index} className="space-y-1">
                  <div className="text-blue-400 font-mono text-xs">{config.project}_service_{service.name}</div>
                  <div className="flex items-center gap-2 group">
                    <div className="text-gray-500 font-mono text-xs overflow-x-auto whitespace-nowrap scrollbar-thin scrollbar-thumb-gray-600 scrollbar-track-transparent">
                      {accountId}.dkr.ecr.{region}.amazonaws.com/{config.project}_service_{service.name}
                    </div>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
                      onClick={() => handleCopyUrl(`${accountId}.dkr.ecr.${region}.amazonaws.com/${config.project}_service_{service.name}`)}
                    >
                      {copiedUrl === `${accountId}.dkr.ecr.${region}.amazonaws.com/${config.project}_service_{service.name}` ? (
                        <Check className="h-3 w-3 text-green-400" />
                      ) : (
                        <Copy className="h-3 w-3" />
                      )}
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
          
          {((config.scheduled_tasks && config.scheduled_tasks.length > 0) || 
            (config.event_processor_tasks && config.event_processor_tasks.length > 0)) && (
            <div className="bg-gray-800 rounded-lg p-3 space-y-2">
              <div className="text-xs font-medium text-gray-300 mb-2">Task Repositories</div>
              {config.scheduled_tasks?.map((task, index) => (
                <div key={`scheduled-${index}`} className="space-y-1">
                  <div className="text-blue-400 font-mono text-xs">{config.project}_task_{task.name}</div>
                  <div className="flex items-center gap-2 group">
                    <div className="text-gray-500 font-mono text-xs overflow-x-auto whitespace-nowrap scrollbar-thin scrollbar-thumb-gray-600 scrollbar-track-transparent">
                      {accountId}.dkr.ecr.{region}.amazonaws.com/{config.project}_task_{task.name}
                    </div>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
                      onClick={() => handleCopyUrl(`${accountId}.dkr.ecr.${region}.amazonaws.com/${config.project}_task_{task.name}`)}
                    >
                      {copiedUrl === `${accountId}.dkr.ecr.${region}.amazonaws.com/${config.project}_task_{task.name}` ? (
                        <Check className="h-3 w-3 text-green-400" />
                      ) : (
                        <Copy className="h-3 w-3" />
                      )}
                    </Button>
                  </div>
                </div>
              ))}
              {config.event_processor_tasks?.map((task, index) => (
                <div key={`event-${index}`} className="space-y-1">
                  <div className="text-blue-400 font-mono text-xs">{config.project}_task_{task.name}</div>
                  <div className="flex items-center gap-2 group">
                    <div className="text-gray-500 font-mono text-xs overflow-x-auto whitespace-nowrap scrollbar-thin scrollbar-thumb-gray-600 scrollbar-track-transparent">
                      {accountId}.dkr.ecr.{region}.amazonaws.com/{config.project}_task_{task.name}
                    </div>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
                      onClick={() => handleCopyUrl(`${accountId}.dkr.ecr.${region}.amazonaws.com/${config.project}_task_{task.name}`)}
                    >
                      {copiedUrl === `${accountId}.dkr.ecr.${region}.amazonaws.com/${config.project}_task_{task.name}` ? (
                        <Check className="h-3 w-3 text-green-400" />
                      ) : (
                        <Copy className="h-3 w-3" />
                      )}
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="mt-4 text-xs text-gray-500">
        Total repositories: {1 + (config.services?.length || 0) + (config.scheduled_tasks?.length || 0) + (config.event_processor_tasks?.length || 0)}
      </div>
    </div>
  );
}
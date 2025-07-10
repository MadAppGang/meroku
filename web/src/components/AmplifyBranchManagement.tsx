import { useState } from 'react';
import { Plus, Trash2, GitBranch, Check, X, Key } from 'lucide-react';
import type { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Button } from './ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { Checkbox } from './ui/checkbox';
import { Textarea } from './ui/textarea';

interface AmplifyBranchManagementProps {
  config: YamlInfrastructureConfig;
  nodeId: string;
  onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function AmplifyBranchManagement({ config, nodeId, onConfigChange }: AmplifyBranchManagementProps) {
  const appName = nodeId.replace('amplify-', '');
  const amplifyAppIndex = config.amplify_apps?.findIndex(app => app.name === appName) ?? -1;
  const amplifyApp = config.amplify_apps?.[amplifyAppIndex];
  
  const [editingBranch, setEditingBranch] = useState<number | null>(null);
  const [newBranch, setNewBranch] = useState({
    name: '',
    stage: 'DEVELOPMENT' as const,
    enable_auto_build: true,
    enable_pull_request_preview: false,
    environment_variables_text: ''
  });
  const [showAddBranch, setShowAddBranch] = useState(false);

  if (!amplifyApp) {
    return (
      <div className="text-gray-400">
        <p>Amplify app configuration not found.</p>
      </div>
    );
  }

  const handleUpdateBranches = (branches: typeof amplifyApp.branches) => {
    if (onConfigChange && config.amplify_apps) {
      const updatedApps = [...config.amplify_apps];
      updatedApps[amplifyAppIndex] = {
        ...amplifyApp,
        branches,
      };
      onConfigChange({ amplify_apps: updatedApps });
    }
  };

  const handleAddBranch = () => {
    if (!newBranch.name) return;
    
    // Parse environment variables
    const envVars: Record<string, string> = {};
    if (newBranch.environment_variables_text) {
      const lines = newBranch.environment_variables_text.split('\n').filter(line => line.trim());
      for (const line of lines) {
        const [key, ...valueParts] = line.split('=');
        if (key?.trim()) {
          envVars[key.trim()] = valueParts.join('=').trim();
        }
      }
    }

    const updatedBranches = [...(amplifyApp.branches || []), {
      name: newBranch.name,
      stage: newBranch.stage,
      enable_auto_build: newBranch.enable_auto_build,
      enable_pull_request_preview: newBranch.enable_pull_request_preview,
      environment_variables: envVars,
      custom_subdomains: [] // Add this field for consistency
    }];

    handleUpdateBranches(updatedBranches);
    
    // Reset form
    setNewBranch({
      name: '',
      stage: 'DEVELOPMENT',
      enable_auto_build: true,
      enable_pull_request_preview: false,
      environment_variables_text: ''
    });
    setShowAddBranch(false);
  };

  const handleDeleteBranch = (index: number) => {
    const updatedBranches = amplifyApp.branches.filter((_, i) => i !== index);
    handleUpdateBranches(updatedBranches);
  };

  const handleUpdateBranch = (index: number, updates: any) => {
    const updatedBranches = [...amplifyApp.branches];
    updatedBranches[index] = {
      ...updatedBranches[index],
      ...updates
    };
    handleUpdateBranches(updatedBranches);
  };

  // Helper component for stage badges
  const StageBadge = ({ stage }: { stage?: string }) => {
    const colorMap: Record<string, string> = {
      PRODUCTION: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
      BETA: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
      DEVELOPMENT: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
      EXPERIMENTAL: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200'
    };
    
    return (
      <span className={`px-2 py-0.5 text-xs font-medium rounded-full ${colorMap[stage || 'DEVELOPMENT']}`}>
        {stage || 'DEVELOPMENT'}
      </span>
    );
  };

  const branches = amplifyApp.branches || [];

  return (
    <div className="space-y-6">
      <div>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-sm font-medium text-white">Branches</h3>
          {!showAddBranch && (
            <Button
              size="sm"
              onClick={() => setShowAddBranch(true)}
            >
              <Plus className="w-4 h-4 mr-1" />
              Add Branch
            </Button>
          )}
        </div>

        {/* Add new branch form */}
        {showAddBranch && (
          <div className="bg-gray-800 rounded-lg p-4 border border-gray-700 mb-4 space-y-3">
            <h4 className="text-sm font-medium text-white">New Branch</h4>
            
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="new-branch-name">Branch Name</Label>
                <Input
                  id="new-branch-name"
                  value={newBranch.name}
                  onChange={(e) => setNewBranch({ ...newBranch, name: e.target.value })}
                  placeholder="feature/new-feature"
                  className="mt-1 bg-gray-900 border-gray-600 text-white"
                />
              </div>
              
              <div>
                <Label htmlFor="new-branch-stage">Stage</Label>
                <Select
                  value={newBranch.stage}
                  onValueChange={(value) => setNewBranch({ ...newBranch, stage: value as any })}
                >
                  <SelectTrigger id="new-branch-stage" className="mt-1 bg-gray-900 border-gray-600 text-white">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="PRODUCTION">Production</SelectItem>
                    <SelectItem value="DEVELOPMENT">Development</SelectItem>
                    <SelectItem value="BETA">Beta</SelectItem>
                    <SelectItem value="EXPERIMENTAL">Experimental</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="new-auto-build"
                  checked={newBranch.enable_auto_build}
                  onCheckedChange={(checked) => 
                    setNewBranch({ ...newBranch, enable_auto_build: checked as boolean })
                  }
                />
                <Label htmlFor="new-auto-build" className="font-normal">
                  Enable automatic builds on push
                </Label>
              </div>
              
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="new-pr-preview"
                  checked={newBranch.enable_pull_request_preview}
                  onCheckedChange={(checked) => 
                    setNewBranch({ ...newBranch, enable_pull_request_preview: checked as boolean })
                  }
                />
                <Label htmlFor="new-pr-preview" className="font-normal">
                  Enable pull request previews
                </Label>
              </div>
            </div>

            <div>
              <Label htmlFor="new-env-vars">Environment Variables</Label>
              <Textarea
                id="new-env-vars"
                value={newBranch.environment_variables_text}
                onChange={(e) => setNewBranch({ ...newBranch, environment_variables_text: e.target.value })}
                placeholder="REACT_APP_API_URL=https://api.example.com&#10;REACT_APP_ENV=production"
                rows={3}
                className="mt-1 bg-gray-900 border-gray-600 text-white"
              />
              <p className="text-xs text-gray-500 mt-1">
                Enter one per line in KEY=VALUE format
              </p>
            </div>

            <div className="flex justify-end gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setShowAddBranch(false);
                  setNewBranch({
                    name: '',
                    stage: 'DEVELOPMENT',
                    enable_auto_build: true,
                    enable_pull_request_preview: false,
                    environment_variables_text: ''
                  });
                }}
              >
                Cancel
              </Button>
              <Button
                size="sm"
                onClick={handleAddBranch}
                disabled={!newBranch.name}
              >
                Add Branch
              </Button>
            </div>
          </div>
        )}

        {/* Branches list */}
        {branches.length === 0 ? (
          <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
            <p className="text-sm text-gray-500">No branches configured. Add a branch to get started.</p>
          </div>
        ) : (
          <div className="space-y-3">
            {branches.map((branch, index) => (
              <div key={index} className="bg-gray-800 rounded-lg p-4 border border-gray-700">
                {editingBranch === index ? (
                  // Edit mode
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <h4 className="text-sm font-medium text-white">Edit Branch</h4>
                      <div className="flex gap-2">
                        <Button
                          size="icon"
                          variant="ghost"
                          onClick={() => setEditingBranch(null)}
                        >
                          <X className="w-4 h-4" />
                        </Button>
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <Label>Branch Name</Label>
                        <Input
                          value={branch.name}
                          onChange={(e) => handleUpdateBranch(index, { name: e.target.value })}
                          className="mt-1 bg-gray-900 border-gray-600 text-white"
                        />
                      </div>
                      
                      <div>
                        <Label>Stage</Label>
                        <Select
                          value={branch.stage || 'DEVELOPMENT'}
                          onValueChange={(value) => handleUpdateBranch(index, { stage: value })}
                        >
                          <SelectTrigger className="mt-1 bg-gray-900 border-gray-600 text-white">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="PRODUCTION">Production</SelectItem>
                            <SelectItem value="DEVELOPMENT">Development</SelectItem>
                            <SelectItem value="BETA">Beta</SelectItem>
                            <SelectItem value="EXPERIMENTAL">Experimental</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    </div>

                    <div className="space-y-2">
                      <div className="flex items-center space-x-2">
                        <Checkbox
                          id={`auto-build-${index}`}
                          checked={branch.enable_auto_build ?? true}
                          onCheckedChange={(checked) => 
                            handleUpdateBranch(index, { enable_auto_build: checked })
                          }
                        />
                        <Label htmlFor={`auto-build-${index}`} className="font-normal">
                          Enable automatic builds
                        </Label>
                      </div>
                      
                      <div className="flex items-center space-x-2">
                        <Checkbox
                          id={`pr-preview-${index}`}
                          checked={branch.enable_pull_request_preview ?? false}
                          onCheckedChange={(checked) => 
                            handleUpdateBranch(index, { enable_pull_request_preview: checked })
                          }
                        />
                        <Label htmlFor={`pr-preview-${index}`} className="font-normal">
                          Enable PR previews
                        </Label>
                      </div>
                    </div>

                    <div>
                      <Label className="text-sm">Environment Variables</Label>
                      <div className="mt-2 space-y-2">
                        {Object.entries(branch.environment_variables || {}).map(([key, value], envIndex) => (
                          <div key={`${key}-${envIndex}`} className="flex gap-2">
                            <Input
                              value={key}
                              onChange={(e) => {
                                const newEnvVars = { ...branch.environment_variables };
                                delete newEnvVars[key];
                                newEnvVars[e.target.value] = value;
                                handleUpdateBranch(index, { environment_variables: newEnvVars });
                              }}
                              placeholder="KEY"
                              className="flex-1 bg-gray-900 border-gray-600 text-white font-mono text-sm"
                            />
                            <Input
                              value={value}
                              onChange={(e) => {
                                const newEnvVars = { ...branch.environment_variables };
                                newEnvVars[key] = e.target.value;
                                handleUpdateBranch(index, { environment_variables: newEnvVars });
                              }}
                              placeholder="VALUE"
                              className="flex-[2] bg-gray-900 border-gray-600 text-white font-mono text-sm"
                            />
                            <Button
                              size="icon"
                              variant="ghost"
                              onClick={() => {
                                const newEnvVars = { ...branch.environment_variables };
                                delete newEnvVars[key];
                                handleUpdateBranch(index, { environment_variables: newEnvVars });
                              }}
                              className="text-red-400 hover:text-red-300"
                            >
                              <X className="w-4 h-4" />
                            </Button>
                          </div>
                        ))}
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => {
                            const newEnvVars = { ...branch.environment_variables, [`NEW_VAR_${Date.now()}`]: '' };
                            handleUpdateBranch(index, { environment_variables: newEnvVars });
                          }}
                          className="w-full"
                        >
                          <Plus className="w-4 h-4 mr-1" />
                          Add Variable
                        </Button>
                      </div>
                      <p className="text-xs text-gray-500 mt-1">
                        Use ${'{'}variable{'}'} for interpolation (e.g., ${'{'}cognito_user_pool_id{'}'})
                      </p>
                    </div>

                    <Button
                      size="sm"
                      onClick={() => setEditingBranch(null)}
                    >
                      <Check className="w-4 h-4 mr-1" />
                      Save
                    </Button>
                  </div>
                ) : (
                  // View mode
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <GitBranch className="w-4 h-4 text-gray-400" />
                        <span className="text-sm font-medium text-white">{branch.name}</span>
                        <StageBadge stage={branch.stage} />
                      </div>
                      <div className="flex gap-2">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setEditingBranch(index)}
                        >
                          Edit
                        </Button>
                        {branches.length > 1 && (
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => handleDeleteBranch(index)}
                            className="text-red-400 hover:text-red-300"
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        )}
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-4 text-sm">
                      <div>
                        <p className="text-gray-400 text-xs">Auto Build</p>
                        <p className="text-gray-300">{branch.enable_auto_build ? 'Enabled' : 'Disabled'}</p>
                      </div>
                      <div>
                        <p className="text-gray-400 text-xs">PR Previews</p>
                        <p className="text-gray-300">{branch.enable_pull_request_preview ? 'Enabled' : 'Disabled'}</p>
                      </div>
                    </div>

                    <div>
                      <p className="text-gray-400 text-xs mb-1">Environment Variables</p>
                      {Object.keys(branch.environment_variables || {}).length > 0 ? (
                        <div className="space-y-1">
                          <p className="text-gray-300 text-sm">
                            {Object.keys(branch.environment_variables || {}).length} variable{Object.keys(branch.environment_variables || {}).length !== 1 ? 's' : ''} configured
                          </p>
                          <div className="flex items-center gap-2 text-xs text-gray-400">
                            <Key className="w-3 h-3" />
                            <span className="truncate">
                              {Object.keys(branch.environment_variables || {}).slice(0, 3).join(', ')}
                              {Object.keys(branch.environment_variables || {}).length > 3 && '...'}
                            </span>
                          </div>
                        </div>
                      ) : (
                        <p className="text-gray-300 text-sm">No variables configured</p>
                      )}
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <div>
        <h3 className="text-sm font-medium text-white mb-4">Branch Requirements</h3>
        <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
          <ul className="text-sm text-gray-300 space-y-2">
            <li className="flex items-start gap-2">
              <span className="text-gray-400">•</span>
              At least one branch is required for deployment
            </li>
            <li className="flex items-start gap-2">
              <span className="text-gray-400">•</span>
              Custom domains require at least one PRODUCTION branch
            </li>
            <li className="flex items-start gap-2">
              <span className="text-gray-400">•</span>
              Branch names must be unique within the app
            </li>
            <li className="flex items-start gap-2">
              <span className="text-gray-400">•</span>
              Environment variables are branch-specific
            </li>
          </ul>
        </div>
      </div>

      <div>
        <h3 className="text-sm font-medium text-white mb-4">Common Environment Variables</h3>
        <div className="bg-gray-800 rounded-lg p-4 border border-gray-700 space-y-3">
          <div>
            <p className="text-xs font-medium text-gray-400 mb-2">React Apps</p>
            <div className="space-y-1">
              <code className="text-xs text-gray-300 block">REACT_APP_API_URL</code>
              <code className="text-xs text-gray-300 block">REACT_APP_ENV</code>
              <code className="text-xs text-gray-300 block">REACT_APP_COGNITO_USER_POOL_ID=${'{'}cognito_user_pool_id{'}'}</code>
            </div>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-400 mb-2">Next.js Apps</p>
            <div className="space-y-1">
              <code className="text-xs text-gray-300 block">NEXT_PUBLIC_API_URL</code>
              <code className="text-xs text-gray-300 block">NEXT_PUBLIC_ENV</code>
            </div>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-400 mb-2">Vite Apps</p>
            <div className="space-y-1">
              <code className="text-xs text-gray-300 block">VITE_API_URL</code>
              <code className="text-xs text-gray-300 block">VITE_ENV</code>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
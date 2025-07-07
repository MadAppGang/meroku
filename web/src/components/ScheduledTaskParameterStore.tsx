import React, { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Textarea } from './ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { Key, Lock, FolderOpen, FileText, Plus, Trash2, Edit2, Check, X, RefreshCw, Eye, EyeOff, AlertCircle, Loader2 } from 'lucide-react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { infrastructureApi, SSMParameterMetadata, SSMParameter } from '../api/infrastructure';
import { Alert, AlertDescription } from './ui/alert';
import { ComponentNode } from '../types';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/dialog';

interface ScheduledTaskParameterStoreProps {
  config: YamlInfrastructureConfig;
  node: ComponentNode;
}

export function ScheduledTaskParameterStore({ config, node }: ScheduledTaskParameterStoreProps) {
  // Extract task name from node id (e.g., "scheduled-daily-report" -> "daily-report")
  const taskName = node.id.replace('scheduled-', '');
  const parameterPath = `/${config.env}/${config.project}/task/${taskName}`;
  
  const [parameters, setParameters] = useState<SSMParameterMetadata[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedParam, setSelectedParam] = useState<SSMParameterMetadata | null>(null);
  const [editingValue, setEditingValue] = useState('');
  const [parameterValue, setParameterValue] = useState('');
  const [loadingValue, setLoadingValue] = useState(false);
  const [savingValue, setSavingValue] = useState(false);
  const [showEditDialog, setShowEditDialog] = useState(false);
  
  // New parameter form state
  const [showNewForm, setShowNewForm] = useState(false);
  const [newParamName, setNewParamName] = useState('');
  const [newParamValue, setNewParamValue] = useState('');
  const [newParamType, setNewParamType] = useState<'String' | 'StringList' | 'SecureString'>('SecureString');
  const [newParamDescription, setNewParamDescription] = useState('');
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    loadParameters();
  }, [parameterPath]);

  const loadParameters = async () => {
    try {
      setLoading(true);
      setError(null);
      const params = await infrastructureApi.listSSMParameters(parameterPath);
      setParameters(params);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load parameters');
    } finally {
      setLoading(false);
    }
  };

  const handleViewParameter = async (param: SSMParameterMetadata) => {
    try {
      setSelectedParam(param);
      setLoadingValue(true);
      setShowEditDialog(true);
      
      const fullParam = await infrastructureApi.getSSMParameter(param.name);
      setParameterValue(fullParam.value);
      setEditingValue(fullParam.value);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load parameter value');
      setShowEditDialog(false);
    } finally {
      setLoadingValue(false);
    }
  };

  const handleSaveParameter = async () => {
    if (!selectedParam) return;
    
    try {
      setSavingValue(true);
      
      await infrastructureApi.createOrUpdateSSMParameter({
        name: selectedParam.name,
        value: editingValue,
        type: selectedParam.type,
        description: selectedParam.description,
        overwrite: true
      });
      
      setParameterValue(editingValue);
      await loadParameters();
      setShowEditDialog(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update parameter');
    } finally {
      setSavingValue(false);
    }
  };

  const handleDeleteParameter = async (name: string) => {
    if (!confirm(`Are you sure you want to delete parameter ${name}?`)) return;
    
    try {
      await infrastructureApi.deleteSSMParameter(name);
      await loadParameters();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete parameter');
    }
  };

  const handleCreateParameter = async () => {
    try {
      setCreating(true);
      const fullName = `${parameterPath}/${newParamName}`;
      
      await infrastructureApi.createOrUpdateSSMParameter({
        name: fullName,
        value: newParamValue,
        type: newParamType,
        description: newParamDescription,
        overwrite: false
      });
      
      setShowNewForm(false);
      setNewParamName('');
      setNewParamValue('');
      setNewParamDescription('');
      await loadParameters();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create parameter');
    } finally {
      setCreating(false);
    }
  };

  const getParamDisplayName = (fullName: string) => {
    return fullName.replace(`${parameterPath}/`, '');
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <FolderOpen className="w-5 h-5" />
            Parameter Namespace
          </CardTitle>
          <CardDescription>
            <code className="font-mono text-sm">{parameterPath}/*</code>
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="p-3 bg-gray-800 rounded">
            <p className="text-sm text-gray-300 mb-2">All parameters in this namespace are automatically:</p>
            <ul className="text-sm text-gray-400 space-y-1 ml-4">
              <li>• Discovered at runtime when the task executes</li>
              <li>• Transformed to uppercase environment variables</li>
              <li>• Injected into the ECS task container as secrets</li>
              <li>• Encrypted using AWS KMS</li>
            </ul>
          </div>
        </CardContent>
      </Card>

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Task Parameters</CardTitle>
              <CardDescription>SSM parameters for task: {taskName}</CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={loadParameters}
                disabled={loading}
              >
                <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              </Button>
              <Button
                size="sm"
                onClick={() => setShowNewForm(true)}
                disabled={showNewForm}
              >
                <Plus className="w-4 h-4 mr-1" />
                Add Parameter
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {loading && !parameters.length ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="w-6 h-6 animate-spin" />
            </div>
          ) : (
            <div className="space-y-3">
              {/* New Parameter Form */}
              {showNewForm && (
                <div className="border border-blue-700 bg-blue-900/10 rounded-lg p-4 space-y-3">
                  <h4 className="text-sm font-medium text-blue-400">Add New Parameter</h4>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label htmlFor="new-param-name" className="text-xs">Parameter Name</Label>
                      <Input
                        id="new-param-name"
                        placeholder="parameter_name"
                        value={newParamName}
                        onChange={(e) => setNewParamName(e.target.value)}
                        className="mt-1 h-8 text-sm"
                      />
                      <p className="text-xs text-gray-500 mt-1">{parameterPath}/{newParamName}</p>
                    </div>
                    <div>
                      <Label htmlFor="new-param-type" className="text-xs">Type</Label>
                      <Select value={newParamType} onValueChange={(v) => setNewParamType(v as any)}>
                        <SelectTrigger className="mt-1 h-8 text-sm">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="String">String</SelectItem>
                          <SelectItem value="StringList">StringList</SelectItem>
                          <SelectItem value="SecureString">SecureString</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div>
                    <Label htmlFor="new-param-value" className="text-xs">Value</Label>
                    <Textarea
                      id="new-param-value"
                      placeholder="Parameter value..."
                      value={newParamValue}
                      onChange={(e) => setNewParamValue(e.target.value)}
                      className="mt-1 h-20 text-sm"
                    />
                  </div>
                  <div>
                    <Label htmlFor="new-param-desc" className="text-xs">Description (optional)</Label>
                    <Input
                      id="new-param-desc"
                      placeholder="What is this parameter for?"
                      value={newParamDescription}
                      onChange={(e) => setNewParamDescription(e.target.value)}
                      className="mt-1 h-8 text-sm"
                    />
                  </div>
                  <div className="flex justify-end gap-2">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        setShowNewForm(false);
                        setNewParamName('');
                        setNewParamValue('');
                        setNewParamDescription('');
                      }}
                      disabled={creating}
                    >
                      Cancel
                    </Button>
                    <Button
                      size="sm"
                      onClick={handleCreateParameter}
                      disabled={!newParamName || !newParamValue || creating}
                    >
                      {creating ? (
                        <Loader2 className="w-4 h-4 animate-spin mr-1" />
                      ) : (
                        <Check className="w-4 h-4 mr-1" />
                      )}
                      Create
                    </Button>
                  </div>
                </div>
              )}

              {/* Parameters List */}
              {parameters.map((param) => {
                const displayName = getParamDisplayName(param.name);
                
                return (
                  <div 
                    key={param.name} 
                    className="border border-gray-700 rounded-lg p-3 space-y-2"
                  >
                    <div className="flex items-start justify-between">
                      <div className="flex items-center gap-2">
                        <Key className="w-4 h-4 text-blue-400" />
                        <code className="text-sm font-mono text-blue-400">
                          {displayName.toUpperCase().replace(/-/g, '_')}
                        </code>
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className="text-xs">
                          {param.type}
                        </Badge>
                      </div>
                    </div>
                    
                    {param.description && (
                      <p className="text-xs text-gray-400">{param.description}</p>
                    )}
                    
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2 text-xs">
                        <FileText className="w-3 h-3 text-gray-500" />
                        <code className="text-gray-500">{param.name}</code>
                      </div>
                      <div className="flex items-center gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => handleViewParameter(param)}
                          className="h-6 px-2 text-xs"
                        >
                          <Eye className="w-3 h-3 mr-1" />
                          View
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => handleDeleteParameter(param.name)}
                          className="h-6 w-6 p-0 text-red-400 hover:text-red-300"
                        >
                          <Trash2 className="w-3 h-3" />
                        </Button>
                      </div>
                    </div>
                  </div>
                );
              })}
              
              {!loading && parameters.length === 0 && (
                <div className="text-center py-8 text-gray-400">
                  <Key className="w-8 h-8 mx-auto mb-2 opacity-50" />
                  <p className="text-sm">No parameters found for this task</p>
                  <p className="text-xs mt-1">Click "Add Parameter" to create one</p>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Parameter View/Edit Dialog */}
      <Dialog open={showEditDialog} onOpenChange={setShowEditDialog}>
        <DialogContent className="max-w-4xl w-[90vw] max-h-[85vh]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Key className="w-4 h-4" />
              {selectedParam && getParamDisplayName(selectedParam.name).toUpperCase().replace(/-/g, '_')}
            </DialogTitle>
            <DialogDescription>
              {selectedParam?.description || 'SSM Parameter'}
            </DialogDescription>
          </DialogHeader>
          
          <div className="space-y-4 py-4">
            {loadingValue ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="w-6 h-6 animate-spin" />
              </div>
            ) : (
              <>
                <div className="space-y-2">
                  <Label>Parameter Path</Label>
                  <code className="text-sm text-gray-400 block p-2 bg-gray-800 rounded">
                    {selectedParam?.name}
                  </code>
                </div>
                
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label>Value</Label>
                    <div className="flex items-center gap-2 text-xs text-gray-400">
                      <Badge variant="outline">{selectedParam?.type}</Badge>
                      {selectedParam && (
                        <span>Version: {selectedParam.version}</span>
                      )}
                    </div>
                  </div>
                  <Textarea
                    value={editingValue}
                    onChange={(e) => setEditingValue(e.target.value)}
                    className="font-mono text-sm min-h-[400px] resize-y"
                    placeholder="Enter parameter value..."
                  />
                </div>
                
                {selectedParam?.lastModifiedDate && (
                  <p className="text-xs text-gray-400">
                    Last modified: {new Date(selectedParam.lastModifiedDate).toLocaleString()}
                  </p>
                )}
              </>
            )}
          </div>
          
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowEditDialog(false)}
              disabled={savingValue}
            >
              Cancel
            </Button>
            <Button
              onClick={handleSaveParameter}
              disabled={savingValue || loadingValue || editingValue === parameterValue}
            >
              {savingValue ? (
                <>
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Check className="w-4 h-4 mr-2" />
                  Save Changes
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
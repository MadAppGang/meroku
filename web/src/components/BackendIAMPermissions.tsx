import React, { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Textarea } from './ui/textarea';
import { Shield, Check, X, AlertCircle, Plus, Trash2, Edit2, ChevronDown, ChevronUp } from 'lucide-react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';

interface BackendIAMPermissionsProps {
  config: YamlInfrastructureConfig;
}

export function BackendIAMPermissions({ config }: BackendIAMPermissionsProps) {
  // Custom policies from YAML config
  const [customPolicies, setCustomPolicies] = useState(config.workload?.policy || []);
  const [showNewPolicy, setShowNewPolicy] = useState(false);
  const [newActions, setNewActions] = useState('');
  const [newResources, setNewResources] = useState('');
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editActions, setEditActions] = useState('');
  const [editResources, setEditResources] = useState('');
  const [expandedPolicies, setExpandedPolicies] = useState<number[]>([]);

  const handleAddPolicy = () => {
    if (newActions && newResources) {
      const actions = newActions.split('\n').filter(a => a.trim());
      const resources = newResources.split('\n').filter(r => r.trim());
      setCustomPolicies([...customPolicies, { actions, resources }]);
      setNewActions('');
      setNewResources('');
      setShowNewPolicy(false);
    }
  };

  const handleUpdatePolicy = (index: number) => {
    const actions = editActions.split('\n').filter(a => a.trim());
    const resources = editResources.split('\n').filter(r => r.trim());
    const updated = [...customPolicies];
    updated[index] = { actions, resources };
    setCustomPolicies(updated);
    setEditingIndex(null);
  };

  const handleDeletePolicy = (index: number) => {
    setCustomPolicies(customPolicies.filter((_, i) => i !== index));
  };

  const toggleExpanded = (index: number) => {
    setExpandedPolicies(prev => 
      prev.includes(index) 
        ? prev.filter(i => i !== index)
        : [...prev, index]
    );
  };

  // Unconditional permissions (always applied)
  const unconditionalPermissions = [
    {
      name: 'CloudWatch Full Access',
      type: 'managed',
      arn: 'arn:aws:iam::aws:policy/CloudWatchFullAccess',
      description: 'Applied to both task role and execution role',
    },
    {
      name: 'Backend S3 Bucket Full Access',
      type: 'custom',
      actions: ['s3:*'],
      resources: [`arn:aws:s3:::${config.project}-backend-${config.env}${config.workload?.bucket_postfix || ''}`, `arn:aws:s3:::${config.project}-backend-${config.env}${config.workload?.bucket_postfix || ''}/*`],
      description: 'Full access to backend bucket and all objects within it',
    },
    {
      name: 'SES Email Sending',
      type: 'custom',
      actions: ['ses:SendEmail', 'ses:SendRawEmail'],
      resources: ['*'],
      description: 'Send emails via Amazon SES',
    },
    {
      name: 'SSM Parameter Store Access',
      type: 'custom',
      actions: ['ssm:GetParameter', 'ssm:GetParameters', 'ssm:GetParametersByPath'],
      resources: [`arn:aws:ssm:*:*:parameter/${config.env}/${config.project}/backend/*`],
      description: 'Read parameters from Parameter Store',
    },
    {
      name: 'X-Ray Write Access',
      type: 'managed',
      arn: 'arn:aws:iam::aws:policy/AWSXrayWriteOnlyAccess',
      description: 'Send traces to AWS X-Ray',
    },
  ];

  // Conditional permissions
  const conditionalPermissions = [
    {
      name: 'SQS Access',
      condition: 'sqs_enable = true',
      enabled: !!config.sqs?.enabled,
      type: 'policy_arn',
      policyArn: config.sqs?.policy_arn,
      description: 'Attaches the policy specified in sqs_policy_arn',
    },
    {
      name: 'ECS Execute Command',
      condition: 'backend_remote_access = true',
      enabled: config.workload?.backend_remote_access !== false,
      type: 'custom',
      actions: [
        'ssmmessages:CreateControlChannel',
        'ssmmessages:CreateDataChannel',
        'ssmmessages:OpenControlChannel',
        'ssmmessages:OpenDataChannel',
      ],
      resources: ['*'],
      description: 'Remote shell access to containers',
    },
    {
      name: 'S3 Environment Files',
      condition: 'env_files_s3 is not empty',
      enabled: (config.workload?.env_files_s3?.length || 0) > 0,
      type: 'custom',
      actions: ['s3:*'],
      resources: config.workload?.env_files_s3?.map(f => 
        `arn:aws:s3:::${f.bucket}/${f.key}`
      ) || [],
      description: 'Access to specific S3 environment files',
    },
    {
      name: 'EFS Access',
      condition: 'backend_efs_mounts is not empty',
      enabled: (config.workload?.efs?.length || 0) > 0,
      type: 'custom',
      actions: [
        'elasticfilesystem:ClientMount',
        'elasticfilesystem:ClientWrite',
        'elasticfilesystem:DescribeMountTargets',
        'elasticfilesystem:ClientRootAccess',
      ],
      resources: config.workload?.efs?.map(efs => 
        `arn:aws:elasticfilesystem:*:*:file-system/${efs.name}`
      ) || [],
      description: 'Mount and access EFS file systems',
    },
  ];

  // Task Execution Role permissions
  const executionRolePermissions = [
    {
      name: 'ECS Task Execution Policy',
      type: 'managed',
      arn: 'arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy',
      description: 'Basic ECS task execution permissions',
    },
    {
      name: 'CloudWatch Full Access',
      type: 'managed',
      arn: 'arn:aws:iam::aws:policy/CloudWatchFullAccess',
      description: 'Write logs and metrics',
    },
    {
      name: 'SSM Parameter Store Access',
      type: 'custom',
      actions: ['ssm:GetParameter', 'ssm:GetParameters', 'ssm:GetParametersByPath'],
      resources: [`arn:aws:ssm:*:*:parameter/${config.env}/${config.project}/backend/*`],
      description: 'Read parameters during task startup',
    },
  ];

  // Add S3 env files to execution role if configured
  if ((config.workload?.env_files_s3?.length || 0) > 0) {
    executionRolePermissions.push({
      name: 'S3 Environment Files Access',
      type: 'custom',
      actions: ['s3:GetObject'],
      resources: config.workload?.env_files_s3?.map(f => 
        `arn:aws:s3:::${f.bucket}/${f.key}`
      ) || [],
      description: 'Read environment files during task startup',
    });
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Shield className="w-5 h-5" />
            IAM Roles
          </CardTitle>
          <CardDescription>
            Roles used by the ECS backend service
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div className="p-3 bg-gray-800 rounded">
              <p className="text-sm font-medium text-gray-200 mb-1">Task Role</p>
              <code className="text-sm font-mono text-blue-400">{config.project}_backend_task_{config.env}</code>
              <p className="text-xs text-gray-400 mt-1">Used by the running container to access AWS services</p>
              <p className="text-xs text-gray-500 mt-1">ARN: <code className="font-mono">arn:aws:iam::ACCOUNT:role/{config.project}_backend_task_{config.env}</code></p>
            </div>
            <div className="p-3 bg-gray-800 rounded">
              <p className="text-sm font-medium text-gray-200 mb-1">Task Execution Role</p>
              <code className="text-sm font-mono text-blue-400">{config.project}_backend_task_execution_{config.env}</code>
              <p className="text-xs text-gray-400 mt-1">Used by ECS to start the task (pull images, write logs)</p>
              <p className="text-xs text-gray-500 mt-1">ARN: <code className="font-mono">arn:aws:iam::ACCOUNT:role/{config.project}_backend_task_execution_{config.env}</code></p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Unconditional Permissions */}
      <Card>
        <CardHeader>
          <CardTitle>Service Permissions</CardTitle>
          <CardDescription>These permissions are always attached to the task role</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {unconditionalPermissions.map((perm, index) => (
              <div key={index} className="border border-gray-700 rounded-lg p-3 space-y-2">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2">
                    <Check className="w-4 h-4 text-green-400" />
                    <span className="font-medium text-sm">{perm.name}</span>
                  </div>
                  <Badge variant={perm.type === 'managed' ? 'default' : 'secondary'} className="text-xs">
                    {perm.type === 'managed' ? 'Managed Policy' : 'Custom Policy'}
                  </Badge>
                </div>
                <p className="text-xs text-gray-400">{perm.description}</p>
                {perm.type === 'managed' ? (
                  <code className="text-xs text-gray-500 font-mono block">{perm.arn}</code>
                ) : (
                  <div className="space-y-2">
                    <div>
                      <p className="text-xs font-medium text-gray-500">Actions:</p>
                      <div className="flex flex-wrap gap-1 mt-1">
                        {perm.actions?.map((action, idx) => (
                          <Badge key={idx} variant="outline" className="text-xs font-mono">
                            {action}
                          </Badge>
                        ))}
                      </div>
                    </div>
                    <div>
                      <p className="text-xs font-medium text-gray-500">Resources:</p>
                      <div className="space-y-1 mt-1">
                        {perm.resources?.map((resource, idx) => (
                          <code key={idx} className="text-xs text-gray-400 block font-mono">
                            {resource}
                          </code>
                        ))}
                      </div>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Custom Policies Section */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Custom IAM Policies</CardTitle>
              <CardDescription>Additional permissions for specific AWS services</CardDescription>
            </div>
            <Button
              size="sm"
              onClick={() => setShowNewPolicy(true)}
              disabled={showNewPolicy}
            >
              <Plus className="w-4 h-4 mr-1" />
              Add Policy
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {/* New Policy Form */}
            {showNewPolicy && (
              <div className="border border-blue-700 bg-blue-900/10 rounded-lg p-4 space-y-3">
                <h4 className="text-sm font-medium text-blue-400">Add New Policy</h4>
                <div className="space-y-3">
                  <div>
                    <Label htmlFor="new-actions" className="text-xs">Actions (one per line)</Label>
                    <Textarea
                      id="new-actions"
                      placeholder="s3:GetObject\ns3:PutObject\ndynamodb:GetItem"
                      value={newActions}
                      onChange={(e) => setNewActions(e.target.value)}
                      className="mt-1 h-24 font-mono text-xs"
                    />
                  </div>
                  <div>
                    <Label htmlFor="new-resources" className="text-xs">Resources (one per line)</Label>
                    <Textarea
                      id="new-resources"
                      placeholder="arn:aws:s3:::my-bucket/*\narn:aws:dynamodb:*:*:table/my-table"
                      value={newResources}
                      onChange={(e) => setNewResources(e.target.value)}
                      className="mt-1 h-24 font-mono text-xs"
                    />
                  </div>
                </div>
                <div className="flex justify-end gap-2">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setShowNewPolicy(false);
                      setNewActions('');
                      setNewResources('');
                    }}
                  >
                    Cancel
                  </Button>
                  <Button
                    size="sm"
                    onClick={handleAddPolicy}
                    disabled={!newActions || !newResources}
                  >
                    <Check className="w-4 h-4 mr-1" />
                    Add Policy
                  </Button>
                </div>
              </div>
            )}

            {/* Custom Policies List */}
            {customPolicies.map((policy, index) => {
              const isEditing = editingIndex === index;
              const isExpanded = expandedPolicies.includes(index);
              
              return (
                <div key={index} className="border border-gray-700 rounded-lg p-3">
                  {isEditing ? (
                    <div className="space-y-3">
                      <div>
                        <Label className="text-xs">Actions</Label>
                        <Textarea
                          value={editActions}
                          onChange={(e) => setEditActions(e.target.value)}
                          className="mt-1 h-24 font-mono text-xs"
                        />
                      </div>
                      <div>
                        <Label className="text-xs">Resources</Label>
                        <Textarea
                          value={editResources}
                          onChange={(e) => setEditResources(e.target.value)}
                          className="mt-1 h-24 font-mono text-xs"
                        />
                      </div>
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setEditingIndex(null)}
                        >
                          <X className="w-3 h-3" />
                        </Button>
                        <Button
                          size="sm"
                          onClick={() => handleUpdatePolicy(index)}
                        >
                          <Check className="w-3 h-3 mr-1" />
                          Save
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div>
                      <div className="flex items-start justify-between mb-2">
                        <div className="flex items-center gap-2">
                          <Shield className="w-4 h-4 text-purple-400" />
                          <span className="text-sm font-medium">Custom Policy {index + 1}</span>
                        </div>
                        <div className="flex items-center gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => toggleExpanded(index)}
                            className="h-6 w-6 p-0"
                          >
                            {isExpanded ? <ChevronUp className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              setEditingIndex(index);
                              setEditActions(policy.actions.join('\n'));
                              setEditResources(policy.resources.join('\n'));
                            }}
                            className="h-6 w-6 p-0"
                          >
                            <Edit2 className="w-3 h-3" />
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => handleDeletePolicy(index)}
                            className="h-6 w-6 p-0 text-red-400 hover:text-red-300"
                          >
                            <Trash2 className="w-3 h-3" />
                          </Button>
                        </div>
                      </div>
                      
                      <div className="space-y-2">
                        <div>
                          <p className="text-xs font-medium text-gray-400 mb-1">Actions:</p>
                          <div className="flex flex-wrap gap-1">
                            {(isExpanded ? policy.actions : policy.actions.slice(0, 3)).map((action, idx) => (
                              <Badge key={idx} variant="secondary" className="text-xs font-mono">
                                {action}
                              </Badge>
                            ))}
                            {!isExpanded && policy.actions.length > 3 && (
                              <Badge variant="outline" className="text-xs">
                                +{policy.actions.length - 3} more
                              </Badge>
                            )}
                          </div>
                        </div>
                        
                        {isExpanded && (
                          <div>
                            <p className="text-xs font-medium text-gray-400 mb-1">Resources:</p>
                            <div className="space-y-1">
                              {policy.resources.map((resource, idx) => (
                                <code key={idx} className="text-xs text-gray-300 block font-mono">
                                  {resource}
                                </code>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
            
            {customPolicies.length === 0 && !showNewPolicy && (
              <div className="text-center py-8 text-gray-400">
                <Shield className="w-8 h-8 mx-auto mb-2 opacity-50" />
                <p className="text-sm">No custom policies configured</p>
                <p className="text-xs mt-1">Click "Add Policy" to grant additional permissions</p>
              </div>
            )}
          </div>
          
          {customPolicies.length > 0 && (
            <div className="mt-4 p-3 bg-yellow-900/20 border border-yellow-700 rounded">
              <p className="text-xs text-yellow-300">
                <AlertCircle className="w-3 h-3 inline mr-1" />
                Custom policies are added to the task role. Ensure resources follow the principle of least privilege.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Optional Permissions */}
      <Card>
        <CardHeader>
          <CardTitle>Optional Permissions</CardTitle>
          <CardDescription>Permissions applied based on configuration</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {conditionalPermissions.map((perm, index) => (
              <div 
                key={index} 
                className={`border rounded-lg p-3 space-y-2 ${
                  perm.enabled ? 'border-gray-700' : 'border-gray-800 opacity-60'
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2">
                    {perm.enabled ? (
                      <Check className="w-4 h-4 text-green-400" />
                    ) : (
                      <X className="w-4 h-4 text-gray-500" />
                    )}
                    <span className="font-medium text-sm">{perm.name}</span>
                  </div>
                  <Badge variant={perm.enabled ? 'default' : 'outline'} className="text-xs">
                    {perm.enabled ? 'Active' : 'Inactive'}
                  </Badge>
                </div>
                <p className="text-xs text-blue-400">Condition: <code className="font-mono">{perm.condition}</code></p>
                <p className="text-xs text-gray-400">{perm.description}</p>
                
                {perm.enabled && (
                  <div className="pt-2">
                    {perm.type === 'policy_arn' && perm.policyArn && (
                      <code className="text-xs text-gray-500 font-mono block">Policy ARN: {perm.policyArn}</code>
                    )}
                    {perm.type === 'custom' && perm.actions && (
                      <div className="space-y-2">
                        <div>
                          <p className="text-xs font-medium text-gray-500">Actions:</p>
                          <div className="flex flex-wrap gap-1 mt-1">
                            {perm.actions.map((action, idx) => (
                              <Badge key={idx} variant="outline" className="text-xs font-mono">
                                {action}
                              </Badge>
                            ))}
                          </div>
                        </div>
                        {perm.resources && perm.resources.length > 0 && (
                          <div>
                            <p className="text-xs font-medium text-gray-500">Resources:</p>
                            <div className="space-y-1 mt-1">
                              {perm.resources.map((resource, idx) => (
                                <code key={idx} className="text-xs text-gray-400 block font-mono">
                                  {resource}
                                </code>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Task Execution Role */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Shield className="w-5 h-5" />
            Task Execution Role Permissions
          </CardTitle>
          <CardDescription>
            <code className="font-mono text-sm">{config.project}_backend_task_execution_{config.env}</code>
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div className="p-3 bg-gray-800 rounded mb-3">
              <p className="text-sm text-gray-300">The execution role is used by ECS to pull images, write logs, and read secrets during task startup.</p>
            </div>
            
            {executionRolePermissions.map((perm, index) => (
              <div key={index} className="border border-gray-700 rounded-lg p-3 space-y-2">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2">
                    <Check className="w-4 h-4 text-green-400" />
                    <span className="font-medium text-sm">{perm.name}</span>
                  </div>
                  <Badge variant={perm.type === 'managed' ? 'default' : 'secondary'} className="text-xs">
                    {perm.type === 'managed' ? 'Managed Policy' : 'Custom Policy'}
                  </Badge>
                </div>
                <p className="text-xs text-gray-400">{perm.description}</p>
                {perm.type === 'managed' ? (
                  <code className="text-xs text-gray-500 font-mono block">{perm.arn}</code>
                ) : (
                  <div className="space-y-2">
                    <div>
                      <p className="text-xs font-medium text-gray-500">Actions:</p>
                      <div className="flex flex-wrap gap-1 mt-1">
                        {perm.actions?.map((action, idx) => (
                          <Badge key={idx} variant="outline" className="text-xs font-mono">
                            {action}
                          </Badge>
                        ))}
                      </div>
                    </div>
                    {perm.resources && (
                      <div>
                        <p className="text-xs font-medium text-gray-500">Resources:</p>
                        <div className="space-y-1 mt-1">
                          {perm.resources.map((resource, idx) => (
                            <code key={idx} className="text-xs text-gray-400 block font-mono">
                              {resource}
                            </code>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
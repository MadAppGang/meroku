import React, { useState } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from './ui/dialog';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Textarea } from './ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { X, Plus } from 'lucide-react';

interface AddEventTaskDialogProps {
  open: boolean;
  onClose: () => void;
  onAdd: (task: any) => void;
  existingTasks: string[];
  availableServices: string[];
}

export function AddEventTaskDialog({ open, onClose, onAdd, existingTasks, availableServices }: AddEventTaskDialogProps) {
  const [formData, setFormData] = useState({
    name: '',
    rule_name: '',
    detail_types: [''],
    sources: [''],
    docker_image: '',
    container_command: '',
    cpu: 256,
    memory: 512,
    environment_variables: '',
  });

  const [errors, setErrors] = useState<Record<string, string>>({});

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    const newErrors: Record<string, string> = {};
    
    if (!formData.name) {
      newErrors.name = 'Task name is required';
    } else if (!/^[a-z0-9-]+$/.test(formData.name)) {
      newErrors.name = 'Task name must contain only lowercase letters, numbers, and hyphens';
    } else if (existingTasks.includes(formData.name)) {
      newErrors.name = 'An event task with this name already exists';
    }
    
    if (!formData.rule_name) {
      newErrors.rule_name = 'Rule name is required';
    }
    
    const validDetailTypes = formData.detail_types.filter(dt => dt.trim());
    if (validDetailTypes.length === 0) {
      newErrors.detail_types = 'At least one detail type is required';
    }
    
    const validSources = formData.sources.filter(s => s.trim());
    if (validSources.length === 0) {
      newErrors.sources = 'At least one source is required';
    }
    
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    
    const task: any = {
      name: formData.name,
      rule_name: formData.rule_name,
      detail_types: validDetailTypes,
      sources: validSources,
      cpu: formData.cpu,
      memory: formData.memory,
    };
    
    if (formData.docker_image) {
      task.docker_image = formData.docker_image;
    }
    
    if (formData.container_command) {
      task.container_command = formData.container_command.split(',').map(cmd => cmd.trim()).filter(cmd => cmd);
    }
    
    if (formData.environment_variables) {
      try {
        const envVars = formData.environment_variables.split('\n')
          .map(line => line.trim())
          .filter(line => line && line.includes('='))
          .reduce((acc, line) => {
            const [key, ...valueParts] = line.split('=');
            acc[key.trim()] = valueParts.join('=').trim();
            return acc;
          }, {} as Record<string, string>);
        
        if (Object.keys(envVars).length > 0) {
          task.environment_variables = envVars;
        }
      } catch (error) {
        newErrors.environment_variables = 'Invalid environment variables format';
        setErrors(newErrors);
        return;
      }
    }
    
    onAdd(task);
    handleClose();
  };

  const handleClose = () => {
    setFormData({
      name: '',
      rule_name: '',
      detail_types: [''],
      sources: [''],
      docker_image: '',
      container_command: '',
      cpu: 256,
      memory: 512,
      environment_variables: '',
    });
    setErrors({});
    onClose();
  };

  const addDetailType = () => {
    setFormData({ ...formData, detail_types: [...formData.detail_types, ''] });
  };

  const removeDetailType = (index: number) => {
    setFormData({
      ...formData,
      detail_types: formData.detail_types.filter((_, i) => i !== index)
    });
  };

  const updateDetailType = (index: number, value: string) => {
    const newDetailTypes = [...formData.detail_types];
    newDetailTypes[index] = value;
    setFormData({ ...formData, detail_types: newDetailTypes });
  };

  const addSource = () => {
    setFormData({ ...formData, sources: [...formData.sources, ''] });
  };

  const removeSource = (index: number) => {
    setFormData({
      ...formData,
      sources: formData.sources.filter((_, i) => i !== index)
    });
  };

  const updateSource = (index: number, value: string) => {
    const newSources = [...formData.sources];
    newSources[index] = value;
    setFormData({ ...formData, sources: newSources });
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Add Event Task</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="name">Task Name</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="process-orders"
              />
              {errors.name && <p className="text-sm text-red-500">{errors.name}</p>}
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="rule_name">EventBridge Rule Name</Label>
              <Input
                id="rule_name"
                value={formData.rule_name}
                onChange={(e) => setFormData({ ...formData, rule_name: e.target.value })}
                placeholder="order-processing-rule"
              />
              {errors.rule_name && <p className="text-sm text-red-500">{errors.rule_name}</p>}
            </div>
            
            <div className="grid gap-2">
              <Label>Event Detail Types</Label>
              {formData.detail_types.map((detailType, index) => (
                <div key={index} className="flex gap-2">
                  <Input
                    value={detailType}
                    onChange={(e) => updateDetailType(index, e.target.value)}
                    placeholder="order.created"
                  />
                  {formData.detail_types.length > 1 && (
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      onClick={() => removeDetailType(index)}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              ))}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={addDetailType}
                className="w-fit"
              >
                <Plus className="h-4 w-4 mr-2" />
                Add Detail Type
              </Button>
              {errors.detail_types && <p className="text-sm text-red-500">{errors.detail_types}</p>}
            </div>
            
            <div className="grid gap-2">
              <Label>Event Sources</Label>
              {formData.sources.map((source, index) => (
                <div key={index} className="flex gap-2">
                  <Select
                    value={source}
                    onValueChange={(value) => updateSource(index, value)}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select a service" />
                    </SelectTrigger>
                    <SelectContent>
                      {availableServices.map(service => (
                        <SelectItem key={service} value={service}>
                          {service}
                        </SelectItem>
                      ))}
                      <SelectItem value="custom">Custom Source</SelectItem>
                    </SelectContent>
                  </Select>
                  {source === 'custom' && (
                    <Input
                      placeholder="Enter custom source"
                      onChange={(e) => updateSource(index, e.target.value)}
                    />
                  )}
                  {formData.sources.length > 1 && (
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      onClick={() => removeSource(index)}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              ))}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={addSource}
                className="w-fit"
              >
                <Plus className="h-4 w-4 mr-2" />
                Add Source
              </Button>
              {errors.sources && <p className="text-sm text-red-500">{errors.sources}</p>}
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="docker_image">Docker Image (optional)</Label>
              <Input
                id="docker_image"
                value={formData.docker_image}
                onChange={(e) => setFormData({ ...formData, docker_image: e.target.value })}
                placeholder="Leave empty to use backend image"
              />
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="container_command">Container Command (comma-separated)</Label>
              <Input
                id="container_command"
                value={formData.container_command}
                onChange={(e) => setFormData({ ...formData, container_command: e.target.value })}
                placeholder="node, scripts/process-event.js"
              />
            </div>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="cpu">CPU (units)</Label>
                <Select
                  value={formData.cpu.toString()}
                  onValueChange={(value) => setFormData({ ...formData, cpu: parseInt(value) })}
                >
                  <SelectTrigger id="cpu">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="256">256 (0.25 vCPU)</SelectItem>
                    <SelectItem value="512">512 (0.5 vCPU)</SelectItem>
                    <SelectItem value="1024">1024 (1 vCPU)</SelectItem>
                    <SelectItem value="2048">2048 (2 vCPU)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              
              <div className="grid gap-2">
                <Label htmlFor="memory">Memory (MB)</Label>
                <Select
                  value={formData.memory.toString()}
                  onValueChange={(value) => setFormData({ ...formData, memory: parseInt(value) })}
                >
                  <SelectTrigger id="memory">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="512">512 MB</SelectItem>
                    <SelectItem value="1024">1 GB</SelectItem>
                    <SelectItem value="2048">2 GB</SelectItem>
                    <SelectItem value="4096">4 GB</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="environment_variables">Environment Variables (KEY=VALUE, one per line)</Label>
              <Textarea
                id="environment_variables"
                value={formData.environment_variables}
                onChange={(e) => setFormData({ ...formData, environment_variables: e.target.value })}
                placeholder="EVENT_PROCESSOR=true&#10;LOG_LEVEL=info"
                rows={4}
              />
              {errors.environment_variables && <p className="text-sm text-red-500">{errors.environment_variables}</p>}
            </div>
          </div>
          
          <DialogFooter>
            <Button type="button" variant="outline" onClick={handleClose}>
              Cancel
            </Button>
            <Button type="submit">Add Task</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
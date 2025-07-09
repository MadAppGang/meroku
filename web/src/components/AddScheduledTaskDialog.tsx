import React, { useState } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from './ui/dialog';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Textarea } from './ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { RadioGroup, RadioGroupItem } from './ui/radio-group';

interface AddScheduledTaskDialogProps {
  open: boolean;
  onClose: () => void;
  onAdd: (task: any) => void;
  existingTasks: string[];
}

export function AddScheduledTaskDialog({ open, onClose, onAdd, existingTasks }: AddScheduledTaskDialogProps) {
  const [formData, setFormData] = useState({
    name: '',
    schedule_type: 'rate',
    rate_value: '1',
    rate_unit: 'hours',
    cron_expression: '',
    docker_image: '',
    container_command: '',
    cpu: 256,
    memory: 512,
    environment_variables: '',
  });

  const [errors, setErrors] = useState<Record<string, string>>({});

  const getScheduleExpression = () => {
    if (formData.schedule_type === 'rate') {
      return `rate(${formData.rate_value} ${formData.rate_unit})`;
    }
    return `cron(${formData.cron_expression})`;
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    const newErrors: Record<string, string> = {};
    
    if (!formData.name) {
      newErrors.name = 'Task name is required';
    } else if (!/^[a-z0-9-]+$/.test(formData.name)) {
      newErrors.name = 'Task name must contain only lowercase letters, numbers, and hyphens';
    } else if (existingTasks.includes(formData.name)) {
      newErrors.name = 'A scheduled task with this name already exists';
    }
    
    if (formData.schedule_type === 'cron' && !formData.cron_expression) {
      newErrors.cron_expression = 'Cron expression is required';
    }
    
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    
    const task: any = {
      name: formData.name,
      schedule: getScheduleExpression(),
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
      schedule_type: 'rate',
      rate_value: '1',
      rate_unit: 'hours',
      cron_expression: '',
      docker_image: '',
      container_command: '',
      cpu: 256,
      memory: 512,
      environment_variables: '',
    });
    setErrors({});
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle>Add Scheduled Task</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="name">Task Name</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setFormData({ ...formData, name: e.target.value })}
                placeholder="daily-cleanup"
              />
              {errors.name && <p className="text-sm text-red-500">{errors.name}</p>}
            </div>
            
            <div className="grid gap-2">
              <Label>Schedule Type</Label>
              <RadioGroup
                value={formData.schedule_type}
                onValueChange={(value: string) => setFormData({ ...formData, schedule_type: value })}
              >
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="rate" id="rate" />
                  <Label htmlFor="rate">Rate-based (e.g., every X hours)</Label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="cron" id="cron" />
                  <Label htmlFor="cron">Cron expression</Label>
                </div>
              </RadioGroup>
            </div>
            
            {formData.schedule_type === 'rate' ? (
              <div className="grid grid-cols-2 gap-4">
                <div className="grid gap-2">
                  <Label htmlFor="rate_value">Every</Label>
                  <Input
                    id="rate_value"
                    type="number"
                    value={formData.rate_value}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setFormData({ ...formData, rate_value: e.target.value })}
                    min="1"
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="rate_unit">Unit</Label>
                  <Select
                    value={formData.rate_unit}
                    onValueChange={(value: string) => setFormData({ ...formData, rate_unit: value })}
                  >
                    <SelectTrigger id="rate_unit">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="minutes">Minutes</SelectItem>
                      <SelectItem value="hours">Hours</SelectItem>
                      <SelectItem value="days">Days</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
            ) : (
              <div className="grid gap-2">
                <Label htmlFor="cron_expression">Cron Expression</Label>
                <Input
                  id="cron_expression"
                  value={formData.cron_expression}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => setFormData({ ...formData, cron_expression: e.target.value })}
                  placeholder="0 0 * * *"
                />
                {errors.cron_expression && <p className="text-sm text-red-500">{errors.cron_expression}</p>}
                <p className="text-sm text-muted-foreground">
                  Format: minute hour day-of-month month day-of-week
                </p>
              </div>
            )}
            
            <div className="grid gap-2">
              <Label htmlFor="docker_image">Docker Image (optional)</Label>
              <Input
                id="docker_image"
                value={formData.docker_image}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setFormData({ ...formData, docker_image: e.target.value })}
                placeholder="Leave empty to use backend image"
              />
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="container_command">Container Command (comma-separated)</Label>
              <Input
                id="container_command"
                value={formData.container_command}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setFormData({ ...formData, container_command: e.target.value })}
                placeholder="node, scripts/cleanup.js"
              />
            </div>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="cpu">CPU (units)</Label>
                <Select
                  value={formData.cpu.toString()}
                  onValueChange={(value: string) => setFormData({ ...formData, cpu: parseInt(value) })}
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
                  onValueChange={(value: string) => setFormData({ ...formData, memory: parseInt(value) })}
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
                onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setFormData({ ...formData, environment_variables: e.target.value })}
                placeholder="TASK_TYPE=cleanup&#10;LOG_LEVEL=info"
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
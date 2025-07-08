import React, { useState } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from './ui/dialog';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Textarea } from './ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';

interface AddServiceDialogProps {
  open: boolean;
  onClose: () => void;
  onAdd: (service: any) => void;
  existingServices: string[];
}

export function AddServiceDialog({ open, onClose, onAdd, existingServices }: AddServiceDialogProps) {
  const [formData, setFormData] = useState({
    name: '',
    docker_image: '',
    container_command: '',
    container_port: 8080,
    cpu: 256,
    memory: 512,
    desired_count: 1,
    health_check_path: '/health',
    environment_variables: '',
  });

  const [errors, setErrors] = useState<Record<string, string>>({});

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    const newErrors: Record<string, string> = {};
    
    if (!formData.name) {
      newErrors.name = 'Service name is required';
    } else if (!/^[a-z0-9-]+$/.test(formData.name)) {
      newErrors.name = 'Service name must contain only lowercase letters, numbers, and hyphens';
    } else if (existingServices.includes(formData.name)) {
      newErrors.name = 'A service with this name already exists';
    }
    
    if (!formData.docker_image) {
      newErrors.docker_image = 'Docker image is required';
    }
    
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    
    const service: any = {
      name: formData.name,
      docker_image: formData.docker_image,
      container_port: formData.container_port,
      cpu: formData.cpu,
      memory: formData.memory,
      desired_count: formData.desired_count,
      health_check_path: formData.health_check_path,
    };
    
    if (formData.container_command) {
      service.container_command = formData.container_command.split(',').map(cmd => cmd.trim()).filter(cmd => cmd);
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
          service.environment_variables = envVars;
        }
      } catch (error) {
        newErrors.environment_variables = 'Invalid environment variables format';
        setErrors(newErrors);
        return;
      }
    }
    
    onAdd(service);
    handleClose();
  };

  const handleClose = () => {
    setFormData({
      name: '',
      docker_image: '',
      container_command: '',
      container_port: 8080,
      cpu: 256,
      memory: 512,
      desired_count: 1,
      health_check_path: '/health',
      environment_variables: '',
    });
    setErrors({});
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle>Add New Service</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="name">Service Name</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="my-service"
              />
              {errors.name && <p className="text-sm text-red-500">{errors.name}</p>}
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="docker_image">Docker Image</Label>
              <Input
                id="docker_image"
                value={formData.docker_image}
                onChange={(e) => setFormData({ ...formData, docker_image: e.target.value })}
                placeholder="nginx:latest"
              />
              {errors.docker_image && <p className="text-sm text-red-500">{errors.docker_image}</p>}
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="container_command">Container Command (comma-separated)</Label>
              <Input
                id="container_command"
                value={formData.container_command}
                onChange={(e) => setFormData({ ...formData, container_command: e.target.value })}
                placeholder="npm, start"
              />
            </div>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="container_port">Container Port</Label>
                <Input
                  id="container_port"
                  type="number"
                  value={formData.container_port}
                  onChange={(e) => setFormData({ ...formData, container_port: parseInt(e.target.value) || 8080 })}
                />
              </div>
              
              <div className="grid gap-2">
                <Label htmlFor="desired_count">Desired Count</Label>
                <Input
                  id="desired_count"
                  type="number"
                  value={formData.desired_count}
                  onChange={(e) => setFormData({ ...formData, desired_count: parseInt(e.target.value) || 1 })}
                  min="0"
                  max="10"
                />
              </div>
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
                    <SelectItem value="4096">4096 (4 vCPU)</SelectItem>
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
                    <SelectItem value="8192">8 GB</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="health_check_path">Health Check Path</Label>
              <Input
                id="health_check_path"
                value={formData.health_check_path}
                onChange={(e) => setFormData({ ...formData, health_check_path: e.target.value })}
              />
            </div>
            
            <div className="grid gap-2">
              <Label htmlFor="environment_variables">Environment Variables (KEY=VALUE, one per line)</Label>
              <Textarea
                id="environment_variables"
                value={formData.environment_variables}
                onChange={(e) => setFormData({ ...formData, environment_variables: e.target.value })}
                placeholder="NODE_ENV=production&#10;PORT=8080"
                rows={4}
              />
              {errors.environment_variables && <p className="text-sm text-red-500">{errors.environment_variables}</p>}
            </div>
          </div>
          
          <DialogFooter>
            <Button type="button" variant="outline" onClick={handleClose}>
              Cancel
            </Button>
            <Button type="submit">Add Service</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
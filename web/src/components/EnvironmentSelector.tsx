import { useEffect, useState } from "react";
import { type Environment, infrastructureApi } from "../api/infrastructure";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";
import { Alert, AlertDescription } from "./ui/alert";
import { Loader2 } from "lucide-react";

interface EnvironmentSelectorProps {
  open: boolean;
  onSelect: (environment: string) => void;
}

export function EnvironmentSelector({ open, onSelect }: EnvironmentSelectorProps) {
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [selectedEnv, setSelectedEnv] = useState<string>("");
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      loadEnvironments();
    }
  }, [open]);

  const loadEnvironments = async () => {
    try {
      setIsLoading(true);
      setError(null);
      
      const envs = await infrastructureApi.getEnvironments();
      
      setEnvironments(envs);
      
      // Check if there's an active environment
      const activeEnv = envs.find(env => env.isActive);
      
      if (activeEnv) {
        // Pre-select the active environment
        setSelectedEnv(activeEnv.name);
      } else if (envs.length > 0) {
        // Otherwise, select the first one
        setSelectedEnv(envs[0].name);
      }
    } catch (error) {
      setError(
        error instanceof Error ? error.message : "Failed to load environments"
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleSelect = () => {
    if (selectedEnv) {
      onSelect(selectedEnv);
    }
  };

  return (
    <Dialog open={open} modal>
      <DialogContent 
        className="sm:max-w-[425px]" 
        onPointerDownOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Select Environment</DialogTitle>
          <DialogDescription>
            Choose an environment to load the infrastructure configuration
          </DialogDescription>
        </DialogHeader>
        
        <div className="py-4 space-y-4">
          {error && (
            <Alert variant="destructive" className="mb-4">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          
          {isLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="size-8 animate-spin" />
            </div>
          ) : environments.length === 0 ? (
            <Alert>
              <AlertDescription>
                No environments found. Please create an environment configuration file first.
              </AlertDescription>
            </Alert>
          ) : (
            <>
              <div>
                <label className="text-sm font-medium mb-2 block">Environment</label>
                <Select value={selectedEnv} onValueChange={setSelectedEnv}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select an environment" />
                  </SelectTrigger>
                  <SelectContent>
                    {environments.map((env) => (
                      <SelectItem key={env.name} value={env.name}>
                        <div className="flex flex-col">
                          <div className="font-medium">{env.name}</div>
                          {env.isActive && (
                            <div className="text-xs text-muted-foreground">
                              Active • {env.profile} • {env.accountId}
                            </div>
                          )}
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </>
          )}
        </div>
        
        <DialogFooter>
          <Button 
            onClick={handleSelect} 
            disabled={!selectedEnv || isLoading}
          >
            Continue
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
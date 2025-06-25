import React from 'react';
import { Zap, Plus, ChevronDown } from 'lucide-react';
import { Button } from '../ui';
import { Environment } from '../../types';

export interface HeaderProps {
  currentEnvironment: Environment;
  onEnvironmentChange: (env: Environment) => void;
  onDeploy: () => void;
}

export const Header: React.FC<HeaderProps> = ({
  currentEnvironment,
  onEnvironmentChange,
  onDeploy
}) => {
  return (
    <header className="bg-gray-800 border-b border-gray-700 px-6 py-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <Zap className="w-6 h-6 text-purple-500" />
            <h1 className="text-xl font-semibold text-white">
              Deployment Platform
            </h1>
          </div>
          
          <div className="flex items-center gap-2 bg-gray-700 px-3 py-1 rounded-lg">
            <span className="text-sm text-gray-300">Environment:</span>
            <select
              className="bg-transparent text-white text-sm outline-none font-medium"
              value={currentEnvironment}
              onChange={(e) => onEnvironmentChange(e.target.value as Environment)}
            >
              <option value="production">production</option>
              <option value="staging">staging</option>
              <option value="development">development</option>
            </select>
            <ChevronDown className="w-4 h-4 text-gray-400" />
          </div>
        </div>
        
        <div className="flex items-center gap-4">
          <nav className="flex items-center gap-4">
            <button className="text-gray-400 hover:text-white font-medium">
              Architecture
            </button>
            <button className="text-gray-400 hover:text-white font-medium">
              Observability
            </button>
            <button className="text-gray-400 hover:text-white font-medium">
              Logs
            </button>
            <button className="text-gray-400 hover:text-white font-medium">
              Settings
            </button>
          </nav>
          
          <Button
            variant="primary"
            icon={<Plus className="w-4 h-4" />}
            onClick={onDeploy}
          >
            Deploy
          </Button>
        </div>
      </div>
    </header>
  );
};
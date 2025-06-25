import React from 'react';
import { Activity, X, Clock, Zap, Database } from 'lucide-react';
import { IconButton } from '../ui';

export interface ActivityPanelProps {
  isOpen: boolean;
  onClose: () => void;
}

export const ActivityPanel: React.FC<ActivityPanelProps> = ({
  isOpen,
  onClose
}) => {
  if (!isOpen) return null;

  const activities = [
    {
      icon: Clock,
      color: 'text-yellow-400',
      title: 'Frontend',
      description: 'Deployment completed • 2 hours ago'
    },
    {
      icon: Zap,
      color: 'text-purple-400',
      title: 'Backend API',
      description: 'Scaled to 3 instances • 1 hour ago'
    },
    {
      icon: Database,
      color: 'text-purple-400',
      title: 'PostgreSQL',
      description: 'Backup completed • 6 hours ago'
    }
  ];

  return (
    <div className="w-80 bg-gray-800 border-l border-gray-700 p-4">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-white font-medium flex items-center gap-2">
          <Activity className="w-4 h-4" />
          Activity
        </h3>
        <IconButton
          icon={<X className="w-4 h-4" />}
          onClick={onClose}
        />
      </div>
      
      <div className="space-y-3">
        {activities.map((activity, index) => {
          const Icon = activity.icon;
          
          return (
            <div key={index} className="text-sm">
              <div className={`flex items-center gap-2 ${activity.color} mb-1`}>
                <Icon className="w-4 h-4" />
                <span>{activity.title}</span>
              </div>
              <p className="text-gray-400 text-xs">{activity.description}</p>
            </div>
          );
        })}
      </div>
    </div>
  );
};
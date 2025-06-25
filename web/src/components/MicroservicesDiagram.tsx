import React from 'react';
import { Check, Database, Package, Globe, Shield, Server } from 'lucide-react';
import './MicroservicesDiagram.css';

interface Service {
  id: string;
  name: string;
  description: string;
  status: 'deployed' | 'pending' | 'error';
  type: 'frontend' | 'backend' | 'database' | 'analytics' | 'gateway';
  position: { x: number; y: number };
  metadata?: string;
}

interface Connection {
  from: string;
  to: string;
}

const services: Service[] = [
  {
    id: 'frontend',
    name: 'frontend',
    description: 'frontend-prod.up.railway.app',
    status: 'deployed',
    type: 'frontend',
    position: { x: 40, y: 20 }
  },
  {
    id: 'backend',
    name: 'backend',
    description: '',
    status: 'deployed',
    type: 'backend',
    position: { x: 65, y: 25 },
    metadata: '3 Replicas'
  },
  {
    id: 'analytics',
    name: 'ackee analytics',
    description: 'ackee-prod.up.railway.app',
    status: 'deployed',
    type: 'analytics',
    position: { x: 15, y: 50 }
  },
  {
    id: 'gateway',
    name: 'api gateway',
    description: 'api-prod.up.railway.app',
    status: 'deployed',
    type: 'gateway',
    position: { x: 40, y: 75 }
  },
  {
    id: 'postgres',
    name: 'postgres',
    description: '',
    status: 'deployed',
    type: 'database',
    position: { x: 65, y: 60 },
    metadata: 'pg-data'
  }
];

const connections: Connection[] = [
  { from: 'frontend', to: 'backend' },
  { from: 'frontend', to: 'analytics' },
  { from: 'frontend', to: 'gateway' },
  { from: 'backend', to: 'postgres' },
  { from: 'gateway', to: 'backend' },
  { from: 'gateway', to: 'postgres' }
];

const getIcon = (type: Service['type']) => {
  switch (type) {
    case 'frontend':
      return <Globe size={24} />;
    case 'backend':
      return <Server size={24} />;
    case 'database':
      return <Database size={24} />;
    case 'analytics':
      return <Shield size={24} />;
    case 'gateway':
      return <Package size={24} />;
  }
};

const getTypeColor = (type: Service['type']) => {
  switch (type) {
    case 'frontend':
      return '#fbbf24';
    case 'backend':
      return '#60a5fa';
    case 'database':
      return '#818cf8';
    case 'analytics':
      return '#34d399';
    case 'gateway':
      return '#f87171';
  }
};

export const MicroservicesDiagram: React.FC = () => {
  const renderConnection = (connection: Connection) => {
    const fromService = services.find(s => s.id === connection.from);
    const toService = services.find(s => s.id === connection.to);
    
    if (!fromService || !toService) return null;

    return (
      <line
        key={`${connection.from}-${connection.to}`}
        x1={`${fromService.position.x}%`}
        y1={`${fromService.position.y}%`}
        x2={`${toService.position.x}%`}
        y2={`${toService.position.y}%`}
        stroke="rgba(100, 100, 100, 0.3)"
        strokeWidth="2"
        strokeDasharray="5,5"
      />
    );
  };

  return (
    <div className="microservices-diagram">
      <div className="diagram-controls">
        <button className="control-btn">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <line x1="8" y1="3" x2="8" y2="13" stroke="currentColor" strokeWidth="2"/>
            <line x1="3" y1="8" x2="13" y2="8" stroke="currentColor" strokeWidth="2"/>
          </svg>
        </button>
        
        <button className="control-btn">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <rect x="3" y="3" width="10" height="10" stroke="currentColor" strokeWidth="2" fill="none"/>
          </svg>
        </button>
      </div>

      <svg className="connections-layer">
        {connections.map(renderConnection)}
      </svg>
      
      <div className="services-layer">
        {services.map(service => (
          <div
            key={service.id}
            className={`microservice-node ${service.status}`}
            style={{
              left: `${service.position.x}%`,
              top: `${service.position.y}%`
            }}
          >
            <div 
              className="service-icon-box"
              style={{ backgroundColor: getTypeColor(service.type) }}
            >
              {getIcon(service.type)}
            </div>
            
            <div className="service-info">
              <h3>{service.name}</h3>
              {service.description && <p className="service-url">{service.description}</p>}
              
              {service.status === 'deployed' && (
                <div className="service-status">
                  <Check size={14} />
                  <span>Deployed just now</span>
                </div>
              )}
              
              {service.metadata && (
                <div className="service-meta">{service.metadata}</div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
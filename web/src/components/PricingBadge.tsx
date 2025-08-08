import { DollarSign } from 'lucide-react';
import { Badge } from './ui/badge';
import type { PricingResponse } from '../hooks/use-pricing';

interface PricingBadgeProps {
  nodeType: string;
  pricing: PricingResponse | null;
  level?: 'startup' | 'scaleup' | 'highload';
  serviceName?: string;
  configProperties?: any;
}

export function PricingBadge({ nodeType, pricing, level = 'startup', serviceName, configProperties }: PricingBadgeProps) {
  if (!pricing) return null;
  
  // Handle both pricing.nodes and direct pricing object structures
  const pricingData = pricing.nodes || pricing;
  
  // Special handling for PostgreSQL/Aurora database pricing
  if (nodeType === 'postgres' && configProperties) {
    if (configProperties.aurora) {
      // Aurora Serverless v2 pricing calculation
      // ACU pricing: ~$0.12 per ACU-hour in us-east-1
      const ACU_HOURLY_PRICE = 0.12;
      const minCapacity = configProperties.minCapacity ?? 0;
      const maxCapacity = configProperties.maxCapacity || 1;
      
      // Calculate based on average expected usage for each level
      let avgACUs = 0;
      if (level === 'startup') {
        // Assume 20% average utilization for startup
        avgACUs = minCapacity + (maxCapacity - minCapacity) * 0.2;
      } else if (level === 'scaleup') {
        // Assume 50% average utilization for scaleup
        avgACUs = minCapacity + (maxCapacity - minCapacity) * 0.5;
      } else {
        // Assume 80% average utilization for highload
        avgACUs = minCapacity + (maxCapacity - minCapacity) * 0.8;
      }
      
      // If min is 0, adjust calculation (database might be paused part of the time)
      if (minCapacity === 0) {
        // Assume database is active 75% of the time for startup, 90% for scaleup, 100% for highload
        const activeTime = level === 'startup' ? 0.75 : level === 'scaleup' ? 0.9 : 1.0;
        avgACUs = avgACUs * activeTime;
      }
      
      const monthlyPrice = avgACUs * ACU_HOURLY_PRICE * 24 * 30; // 24 hours * 30 days
      
      return (
        <Badge 
          variant="secondary" 
          className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
        >
          <DollarSign className="w-3 h-3 mr-0.5" />
          ${monthlyPrice < 1 ? monthlyPrice.toFixed(2) : monthlyPrice.toFixed(0)}/mo
        </Badge>
      );
    }
    // For standard RDS, use the existing pricing
  }

  // Map node types to pricing keys (matching API response keys)
  const pricingMap: Record<string, string> = {
    'ecs': 'vpc', // Show VPC pricing on ECS node
    'backend': 'backend', // Backend service pricing
    'service': 'ecs',
    'postgres': 'rds',
    'aurora': 'rds',
    's3': 's3',
    'cloudwatch': 'cloudwatch',
    'cognito': 'cognito',
    'alb': 'alb',
    'nat_gateway': 'nat_gateway',
    'api-gateway': 'api_gateway', // Fixed: api-gateway -> api_gateway
    'eventbridge': 'eventbridge',
    'lambda': 'lambda',
    'ses': 'ses',
    'sqs': 'sqs',
    'ssm': 'ssm',
    'secrets': 'secrets',
    'route53': 'route53', // Added route53
    'ecr': 'ecr', // Added ecr
    'scheduled-task': 'scheduled', // Added scheduled-task (handled specially above)
    'event-task': 'event', // Added event-task
    'xray': 'xray',
    'efs': 'efs',
    'sns': 'sns',
    'waf': 'waf',
    'secrets-manager': 'secrets',
  };

  // Special handling for backend service
  if (nodeType === 'backend' && serviceName === 'Backend service') {
    const backendPrice = pricingData['backend'];
    if (backendPrice && backendPrice.levels[level]) {
      return (
        <Badge 
          variant="secondary" 
          className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
        >
          <DollarSign className="w-3 h-3 mr-0.5" />
          ${backendPrice.levels[level].monthlyPrice.toFixed(0)}/mo
        </Badge>
      );
    }
  }

  // Special handling for scheduled tasks
  if (nodeType === 'scheduled-task' && serviceName) {
    const scheduledKey = `scheduled_${serviceName.toLowerCase()}`;
    if (pricingData[scheduledKey]) {
      const price = pricingData[scheduledKey].levels[level];
      if (price) {
        // For scheduled tasks, show more precision since costs are typically small
        const monthlyPrice = price.monthlyPrice;
        const displayPrice = monthlyPrice < 1 
          ? `$${monthlyPrice.toFixed(2)}/mo`
          : `$${monthlyPrice.toFixed(0)}/mo`;
        
        return (
          <Badge 
            variant="secondary" 
            className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
          >
            <DollarSign className="w-3 h-3 mr-0.5" />
            {displayPrice}
          </Badge>
        );
      }
    }
  }

  // For other services, check if there's a specific pricing entry
  if (nodeType === 'service' && serviceName) {
    const serviceKey = serviceName.toLowerCase().replace(/-/g, '_').replace(/ /g, '_');
    if (pricingData[serviceKey]) {
      const price = pricingData[serviceKey].levels[level];
      if (price) {
        return (
          <Badge 
            variant="secondary" 
            className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
          >
            <DollarSign className="w-3 h-3 mr-0.5" />
            ${price.monthlyPrice.toFixed(0)}/mo
          </Badge>
        );
      }
    }
  }

  // Use the type mapping
  const pricingKey = pricingMap[nodeType];
  
  // If we have a mapping but no pricing data, show placeholder
  if (pricingKey && !pricingData[pricingKey]) {
    // Show placeholder for mapped services without pricing
    return (
      <Badge 
        variant="secondary" 
        className="absolute -top-2 -right-2 bg-gray-600/90 text-gray-300 border-gray-700 text-xs px-1 py-0.5"
      >
        <DollarSign className="w-3 h-3 mr-0.5" />
        --.--/mo
      </Badge>
    );
  }
  
  if (!pricingKey) return null;

  const price = pricingData[pricingKey]?.levels[level];
  if (!price) {
    // Show placeholder if pricing key exists but no price for this level
    return (
      <Badge 
        variant="secondary" 
        className="absolute -top-2 -right-2 bg-gray-600/90 text-gray-300 border-gray-700 text-xs px-1 py-0.5"
      >
        <DollarSign className="w-3 h-3 mr-0.5" />
        --.--/mo
      </Badge>
    );
  }

  return (
    <Badge 
      variant="secondary" 
      className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
    >
      <DollarSign className="w-3 h-3 mr-0.5" />
      ${price.monthlyPrice.toFixed(0)}/mo
    </Badge>
  );
}
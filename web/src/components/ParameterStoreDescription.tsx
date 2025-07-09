import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Lightbulb, Code, Key, Settings, Info } from 'lucide-react';

interface ParameterStoreDescriptionProps {
  config: YamlInfrastructureConfig;
}

export function ParameterStoreDescription({ config }: ParameterStoreDescriptionProps) {
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>How Auto-Discovery Works</CardTitle>
          <CardDescription>
            Understanding the Parameter Store auto-discovery mechanism
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* What This Means in Practice */}
          <div>
            <h3 className="text-sm font-medium text-gray-300 mb-3 flex items-center gap-2">
              <Lightbulb className="w-4 h-4 text-yellow-400" />
              What This Means in Practice
            </h3>
            
            <div className="space-y-4">
              <div className="bg-gray-800 rounded-lg p-4">
                <h4 className="text-xs font-medium text-gray-300 mb-3">Example Scenario:</h4>
                <ol className="space-y-3 text-xs text-gray-400">
                  <li className="flex items-start gap-2">
                    <span className="text-blue-400 font-medium">1.</span>
                    <span>Your backend service runs with path <code className="text-blue-300">/{config.env}/{config.project}/backend/</code></span>
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="text-blue-400 font-medium">2.</span>
                    <span>You manually add a parameter: <code className="text-blue-300">/{config.env}/{config.project}/backend/stripe_api_key</code></span>
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="text-blue-400 font-medium">3.</span>
                    <div>
                      <span>Auto-discovery happens:</span>
                      <ul className="mt-2 ml-4 space-y-1">
                        <li>• Terraform queries all parameters under <code className="text-blue-300">/{config.env}/{config.project}/backend/</code></li>
                        <li>• Finds: <code>env</code>, <code>pg_database_password</code>, <code>gcm-server-key</code>, <code>stripe_api_key</code></li>
                        <li>• Transforms names: <code>ENV</code>, <code>PG_DATABASE_PASSWORD</code>, <code>GCM_SERVER_KEY</code>, <code>STRIPE_API_KEY</code></li>
                      </ul>
                    </div>
                  </li>
                  <li className="flex items-start gap-2">
                    <span className="text-blue-400 font-medium">4.</span>
                    <span>ECS Container gets environment variables:</span>
                  </li>
                </ol>
                
                <pre className="mt-3 text-xs text-gray-300 bg-gray-900 p-3 rounded overflow-x-auto">
{`# Inside your backend container:
echo $PG_DATABASE_PASSWORD  # → actual database password
echo $STRIPE_API_KEY        # → your manually added key
echo $GCM_SERVER_KEY        # → FCM key (if enabled)`}</pre>
              </div>
            </div>
          </div>

          {/* Key Benefits */}
          <div>
            <h3 className="text-sm font-medium text-gray-300 mb-3 flex items-center gap-2">
              <Key className="w-4 h-4 text-green-400" />
              Key Benefits
            </h3>
            <div className="bg-green-900/20 border border-green-700 rounded-lg p-4">
              <ol className="space-y-2 text-xs text-gray-300">
                <li className="flex items-start gap-2">
                  <span className="text-green-400 font-medium">1.</span>
                  <span><strong>Zero Code Changes:</strong> Add new secrets without modifying Terraform</span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-green-400 font-medium">2.</span>
                  <span><strong>Automatic Injection:</strong> All parameters in the namespace become environment variables</span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-green-400 font-medium">3.</span>
                  <span><strong>Consistent Naming:</strong> Path → Environment variable name transformation</span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-green-400 font-medium">4.</span>
                  <span><strong>Security:</strong> Uses ECS secrets field (encrypted in transit/rest)</span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-green-400 font-medium">5.</span>
                  <span><strong>Flexibility:</strong> Each service gets its own parameter namespace</span>
                </li>
              </ol>
            </div>
          </div>

          {/* Real Example */}
          <div>
            <h3 className="text-sm font-medium text-gray-300 mb-3 flex items-center gap-2">
              <Code className="w-4 h-4 text-purple-400" />
              Real Example
            </h3>
            <div className="bg-purple-900/20 border border-purple-700 rounded-lg p-4">
              <p className="text-xs text-gray-300 mb-3">If you have these parameters:</p>
              <div className="bg-gray-800 rounded p-3 mb-3">
                <code className="text-xs text-purple-300 whitespace-pre">
{`/${config.env}/${config.project}/backend/env
/${config.env}/${config.project}/backend/pg_database_password
/${config.env}/${config.project}/backend/jwt_secret
/${config.env}/${config.project}/backend/redis_url`}</code>
              </div>
              
              <p className="text-xs text-gray-300 mb-3">Your backend container automatically gets:</p>
              <div className="bg-gray-800 rounded p-3">
                <code className="text-xs text-green-300 whitespace-pre">
{`ENV=...
PG_DATABASE_PASSWORD=...
JWT_SECRET=...
REDIS_URL=...`}</code>
              </div>
            </div>
          </div>

          {/* Auto-discovery note */}
          <div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-blue-400 mt-0.5" />
              <div className="flex-1">
                <p className="text-xs text-gray-300">
                  This is why it's called <strong>"auto-discovery"</strong> - the system automatically discovers and injects 
                  ALL parameters in the service's namespace without requiring code changes!
                </p>
              </div>
            </div>
          </div>

          {/* Service-specific management */}
          <div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Settings className="w-4 h-4 text-yellow-400 mt-0.5" />
              <div className="flex-1">
                <h4 className="text-sm font-medium text-yellow-400 mb-2">Service-Specific Management</h4>
                <p className="text-xs text-gray-300">
                  You can manage parameters in detail for each service by clicking on the service node in the infrastructure 
                  diagram. Each service (Backend, Additional Services, Tasks) has its own parameter namespace where you can 
                  add, update, or remove configuration values.
                </p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
import { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Switch } from './ui/switch';
import { Button } from './ui/button';
import { Alert, AlertDescription } from './ui/alert';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { infrastructureApi } from '../api/infrastructure';
import { 
  Database, 
  Shield, 
  Key,
  CheckCircle,
  Info,
  AlertCircle,
  ExternalLink,
  Server,
  FileText,
  Eye,
  EyeOff,
  RefreshCw,
  Copy
} from 'lucide-react';

interface PostgresNodePropertiesProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
  accountInfo?: { accountId: string; region: string; profile: string };
}

export function PostgresNodeProperties({ config, onConfigChange }: PostgresNodePropertiesProps) {
  const postgresConfig = config.postgres || { enabled: false };
  const workloadConfig = config.workload || {};
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [passwordValue, setPasswordValue] = useState<string | null>(null);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  
  const handleTogglePostgres = (enabled: boolean) => {
    onConfigChange({
      postgres: {
        ...postgresConfig,
        enabled
      }
    });
  };

  const handleUpdateConfig = (updates: Partial<typeof postgresConfig>) => {
    onConfigChange({
      postgres: {
        ...postgresConfig,
        ...updates
      }
    });
  };

  const handleTogglePgAdmin = (enabled: boolean) => {
    onConfigChange({
      workload: {
        ...workloadConfig,
        install_pg_admin: enabled
      }
    });
  };

  const handleUpdatePgAdminEmail = (email: string) => {
    onConfigChange({
      workload: {
        ...workloadConfig,
        pg_admin_email: email
      }
    });
  };

  const fetchPassword = async () => {
    setPasswordLoading(true);
    setPasswordError(null);
    
    try {
      const parameter = await infrastructureApi.getSSMParameter(
        `/${config.env}/${config.project}/postgres_password`
      );
      setPasswordValue(parameter.value);
    } catch (error: any) {
      setPasswordError(error.message || 'Failed to fetch password');
    } finally {
      setPasswordLoading(false);
    }
  };

  const [pgAdminPasswordVisible, setPgAdminPasswordVisible] = useState(false);
  const [pgAdminPasswordLoading, setPgAdminPasswordLoading] = useState(false);
  const [pgAdminPasswordValue, setPgAdminPasswordValue] = useState<string | null>(null);
  const [pgAdminPasswordError, setPgAdminPasswordError] = useState<string | null>(null);

  const fetchPgAdminPassword = async () => {
    setPgAdminPasswordLoading(true);
    setPgAdminPasswordError(null);
    
    try {
      const parameter = await infrastructureApi.getSSMParameter(
        `/${config.env}/${config.project}/pgadmin_password`
      );
      setPgAdminPasswordValue(parameter.value);
    } catch (error: any) {
      setPgAdminPasswordError(error.message || 'Failed to fetch pgAdmin password');
    } finally {
      setPgAdminPasswordLoading(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  // Determine actual values with defaults
  const actualDbName = postgresConfig.dbname || config.project;
  const actualUsername = postgresConfig.username || 'postgres';

  return (
    <div className="space-y-6">
      {/* Enable/Disable PostgreSQL */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Database className="w-5 h-5" />
            PostgreSQL Database
          </CardTitle>
          <CardDescription>
            AWS RDS Aurora PostgreSQL Serverless v2 cluster
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <Label htmlFor="postgres-enabled" className="text-base">Enable PostgreSQL</Label>
              <p className="text-sm text-gray-500">
                Create a managed PostgreSQL database cluster
              </p>
            </div>
            <Switch
              id="postgres-enabled"
              checked={postgresConfig.enabled}
              onCheckedChange={handleTogglePostgres}
            />
          </div>
        </CardContent>
      </Card>

      {postgresConfig.enabled && (
        <>
          {/* Database Configuration */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Server className="w-5 h-5" />
                Database Configuration
              </CardTitle>
              <CardDescription>
                Configure your PostgreSQL database settings
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="db-name">Database Name</Label>
                  <Input
                    id="db-name"
                    value={postgresConfig.dbname || ''}
                    onChange={(e) => handleUpdateConfig({ dbname: e.target.value })}
                    placeholder={config.project}
                  />
                  <p className="text-xs text-gray-500">
                    Default: {config.project}
                  </p>
                </div>
                
                <div className="space-y-2">
                  <Label htmlFor="db-username">Master Username</Label>
                  <Input
                    id="db-username"
                    value={postgresConfig.username || ''}
                    onChange={(e) => handleUpdateConfig({ username: e.target.value })}
                    placeholder="postgres"
                  />
                  <p className="text-xs text-gray-500">
                    Default: postgres
                  </p>
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="engine-version">PostgreSQL Version</Label>
                <select
                  id="engine-version"
                  value={postgresConfig.engine_version || '14'}
                  onChange={(e) => handleUpdateConfig({ engine_version: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-800 border border-gray-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                >
                  <option value="16">PostgreSQL 16.x (Latest)</option>
                  <option value="15">PostgreSQL 15.x</option>
                  <option value="14">PostgreSQL 14.x</option>
                  <option value="13">PostgreSQL 13.x</option>
                </select>
              </div>

              <div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
                <div className="space-y-1">
                  <Label className="text-sm font-medium">Public Access</Label>
                  <p className="text-xs text-gray-400">Allow connections from outside VPC</p>
                </div>
                <Switch
                  checked={postgresConfig.public_access || false}
                  onCheckedChange={(checked) => handleUpdateConfig({ public_access: checked })}
                />
              </div>

              {postgresConfig.public_access && (
                <Alert className="border-yellow-600 bg-yellow-50">
                  <AlertCircle className="h-4 w-4 text-yellow-600" />
                  <AlertDescription>
                    Enabling public access exposes your database to the internet. Ensure proper security groups and strong passwords.
                  </AlertDescription>
                </Alert>
              )}
            </CardContent>
          </Card>

          {/* Connection Details */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Key className="w-5 h-5" />
                Connection Details
              </CardTitle>
              <CardDescription>
                How to connect to your database
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-3">
                <div className="bg-gray-800 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-gray-200 mb-2">Environment Variable</h4>
                  <code className="text-xs text-blue-400">DATABASE_URL</code>
                  <p className="text-xs text-gray-500 mt-1">
                    Automatically injected into backend service
                  </p>
                </div>

                <div className="bg-gray-800 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-gray-200 mb-2">Connection String Format</h4>
                  <code className="text-xs text-gray-400 break-all">
                    postgresql://{actualUsername}:[PASSWORD]@[ENDPOINT]:5432/{actualDbName}
                  </code>
                </div>

                <div className="bg-gray-800 rounded-lg p-3">
                  <h4 className="text-sm font-medium text-gray-200 mb-2">Password Location</h4>
                  <div className="space-y-1">
                    <code className="text-xs text-gray-400 block">
                      /{config.env}/{config.project}/postgres_password
                    </code>
                    <code className="text-xs text-gray-400 block">
                      /{config.env}/{config.project}/backend/pg_database_password
                    </code>
                  </div>
                  <p className="text-xs text-gray-500 mt-1">
                    Stored in AWS Systems Manager Parameter Store
                  </p>
                  
                  {/* Password Viewer */}
                  <div className="mt-3 space-y-2">
                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={fetchPassword}
                        disabled={passwordLoading}
                        className="text-xs"
                      >
                        {passwordLoading ? (
                          <>
                            <RefreshCw className="w-3 h-3 mr-1 animate-spin" />
                            Loading...
                          </>
                        ) : (
                          <>
                            <Key className="w-3 h-3 mr-1" />
                            Fetch Password
                          </>
                        )}
                      </Button>
                      
                      {passwordValue && (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setPasswordVisible(!passwordVisible)}
                          className="text-xs"
                        >
                          {passwordVisible ? (
                            <EyeOff className="w-3 h-3" />
                          ) : (
                            <Eye className="w-3 h-3" />
                          )}
                        </Button>
                      )}
                      
                      {passwordValue && (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => copyToClipboard(passwordValue)}
                          className="text-xs"
                        >
                          <Copy className="w-3 h-3" />
                        </Button>
                      )}
                    </div>
                    
                    {passwordError && (
                      <div className="text-xs text-red-400 bg-red-900/20 border border-red-700 rounded p-2">
                        {passwordError}
                      </div>
                    )}
                    
                    {passwordValue && (
                      <div className="text-xs bg-gray-900 p-2 rounded border">
                        {passwordVisible ? (
                          <span className="font-mono text-green-400">{passwordValue}</span>
                        ) : (
                          <span className="font-mono text-gray-400">{'•'.repeat(20)}</span>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              </div>

              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription>
                  The database password is automatically generated and stored securely. Access it via AWS SSM Parameter Store.
                </AlertDescription>
              </Alert>
            </CardContent>
          </Card>

          {/* pgAdmin Configuration */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileText className="w-5 h-5" />
                pgAdmin Interface
              </CardTitle>
              <CardDescription>
                Web-based PostgreSQL administration tool
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between p-3 bg-gray-800 rounded-lg">
                <div className="space-y-1">
                  <Label className="text-sm font-medium">Enable pgAdmin</Label>
                  <p className="text-xs text-gray-400">Deploy pgAdmin container in ECS</p>
                </div>
                <Switch
                  checked={workloadConfig.install_pg_admin || false}
                  onCheckedChange={handleTogglePgAdmin}
                />
              </div>

              {workloadConfig.install_pg_admin && (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="pgadmin-email">pgAdmin Login Email</Label>
                    <Input
                      id="pgadmin-email"
                      type="email"
                      value={workloadConfig.pg_admin_email || ''}
                      onChange={(e) => handleUpdatePgAdminEmail(e.target.value)}
                      placeholder="admin@example.com"
                    />
                    <p className="text-xs text-gray-500">
                      Default: admin@madappgang.com
                    </p>
                  </div>

                  {/* pgAdmin Password Section */}
                  <div className="bg-gray-800 rounded-lg p-3">
                    <h4 className="text-sm font-medium text-gray-200 mb-2">pgAdmin Password</h4>
                    <p className="text-xs text-gray-500 mb-3">
                      Stored in Parameter Store: <code>/{config.env}/{config.project}/pgadmin_password</code>
                    </p>
                    
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={fetchPgAdminPassword}
                          disabled={pgAdminPasswordLoading}
                          className="text-xs"
                        >
                          {pgAdminPasswordLoading ? (
                            <>
                              <RefreshCw className="w-3 h-3 mr-1 animate-spin" />
                              Loading...
                            </>
                          ) : (
                            <>
                              <Key className="w-3 h-3 mr-1" />
                              Fetch Password
                            </>
                          )}
                        </Button>
                        
                        {pgAdminPasswordValue && (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setPgAdminPasswordVisible(!pgAdminPasswordVisible)}
                            className="text-xs"
                          >
                            {pgAdminPasswordVisible ? (
                              <EyeOff className="w-3 h-3" />
                            ) : (
                              <Eye className="w-3 h-3" />
                            )}
                          </Button>
                        )}
                        
                        {pgAdminPasswordValue && (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => copyToClipboard(pgAdminPasswordValue)}
                            className="text-xs"
                          >
                            <Copy className="w-3 h-3" />
                          </Button>
                        )}
                      </div>
                      
                      {pgAdminPasswordError && (
                        <div className="text-xs text-red-400 bg-red-900/20 border border-red-700 rounded p-2">
                          {pgAdminPasswordError}
                        </div>
                      )}
                      
                      {pgAdminPasswordValue && (
                        <div className="text-xs bg-gray-900 p-2 rounded border">
                          {pgAdminPasswordVisible ? (
                            <span className="font-mono text-green-400">{pgAdminPasswordValue}</span>
                          ) : (
                            <span className="font-mono text-gray-400">{'•'.repeat(20)}</span>
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                </>
              )}
            </CardContent>
          </Card>

          {/* Resources Created */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Shield className="w-5 h-5" />
                AWS Resources Created
              </CardTitle>
              <CardDescription>
                Resources that will be created when PostgreSQL is enabled
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="grid grid-cols-1 gap-3">
                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">Aurora Serverless v2 Cluster</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        Auto-scaling PostgreSQL cluster (0.5 - 1 ACU)
                      </p>
                    </div>
                  </div>

                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">Automated Backups</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        7-day retention with point-in-time recovery
                      </p>
                    </div>
                  </div>

                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">Encryption</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        At-rest encryption and SSL/TLS for connections
                      </p>
                    </div>
                  </div>

                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">High Availability</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        Multi-AZ deployment for automatic failover
                      </p>
                    </div>
                  </div>

                  <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                    <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                    <div className="flex-1">
                      <h4 className="text-sm font-medium text-gray-200">Performance Insights</h4>
                      <p className="text-xs text-gray-400 mt-1">
                        Database performance monitoring and analysis
                      </p>
                    </div>
                  </div>

                  {workloadConfig.install_pg_admin && (
                    <div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
                      <CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
                      <div className="flex-1">
                        <h4 className="text-sm font-medium text-gray-200">pgAdmin Container</h4>
                        <p className="text-xs text-gray-400 mt-1">
                          Web interface for database management
                        </p>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Important Notes */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Info className="w-5 h-5" />
                Important Notes
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="space-y-2 text-sm text-gray-300">
                <p>• Database endpoint is automatically provided to backend via DATABASE_URL</p>
                <p>• Password is auto-generated and stored in SSM Parameter Store</p>
                <p>• Aurora Serverless v2 scales automatically based on workload</p>
                <p>• Minimum capacity is 0.5 ACU (can scale to zero when idle)</p>
                <p>• Database is created in private subnets by default</p>
              </div>

              <div className="pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => window.open(`https://console.aws.amazon.com/rds/home?region=${config.region}#databases:`, '_blank')}
                >
                  <ExternalLink className="w-4 h-4 mr-2" />
                  Open RDS Console
                </Button>
              </div>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
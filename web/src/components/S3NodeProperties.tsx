import React, { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Alert, AlertDescription } from './ui/alert';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Switch } from './ui/switch';
import { Textarea } from './ui/textarea';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { 
  HardDrive,
  Globe,
  Lock,
  Plus,
  Trash2,
  ExternalLink,
  AlertCircle,
  FileText,
  Shield,
  History,
  Zap,
  X,
  Edit2,
  Check
} from 'lucide-react';

interface S3NodePropertiesProps {
  config: YamlInfrastructureConfig;
  onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

interface BucketConfig {
  name: string;
  public?: boolean;
  versioning?: boolean;
  cors_rules?: Array<{
    allowed_headers?: string[];
    allowed_methods?: string[];
    allowed_origins?: string[];
    expose_headers?: string[];
    max_age_seconds?: number;
  }>;
}

export function S3NodeProperties({ config, onConfigChange }: S3NodePropertiesProps) {
  const [showNewBucket, setShowNewBucket] = useState(false);
  const [newBucketName, setNewBucketName] = useState('');
  const [newBucketPublic, setNewBucketPublic] = useState(false);
  const [newBucketVersioning, setNewBucketVersioning] = useState(true);
  const [newBucketCors, setNewBucketCors] = useState('');
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editBucket, setEditBucket] = useState<BucketConfig>({ name: '' });
  const [editBucketCors, setEditBucketCors] = useState('');

  // Default CORS configuration for public buckets
  const defaultPublicCors = {
    allowed_origins: ["*"],
    allowed_methods: ["GET", "HEAD"],
    allowed_headers: ["*"],
    expose_headers: ["ETag"],
    max_age_seconds: 3600
  };

  const buckets = config.buckets || [];

  const handleAddBucket = () => {
    if (!newBucketName) return;

    const newBucket: BucketConfig = {
      name: newBucketName,
      public: newBucketPublic,
      versioning: newBucketVersioning
    };

    // Parse CORS if provided
    if (newBucketCors.trim()) {
      try {
        const corsRules = JSON.parse(newBucketCors);
        newBucket.cors_rules = Array.isArray(corsRules) ? corsRules : [corsRules];
      } catch (e) {
        // Invalid JSON, skip CORS
      }
    } else if (newBucketPublic) {
      // Add default CORS for public buckets if no custom CORS provided
      newBucket.cors_rules = [defaultPublicCors];
    }

    onConfigChange({
      buckets: [...buckets, newBucket]
    });

    // Reset form
    setNewBucketName('');
    setNewBucketPublic(false);
    setNewBucketVersioning(true);
    setNewBucketCors('');
    setShowNewBucket(false);
  };

  const handleUpdateBucket = (index: number) => {
    const updatedBuckets = [...buckets];
    
    // Parse CORS if provided
    if (editBucketCors.trim()) {
      try {
        const corsRules = JSON.parse(editBucketCors);
        editBucket.cors_rules = Array.isArray(corsRules) ? corsRules : [corsRules];
      } catch (e) {
        // Invalid JSON, keep existing CORS
      }
    } else {
      // Clear CORS if empty
      delete editBucket.cors_rules;
    }
    
    updatedBuckets[index] = editBucket;
    onConfigChange({ buckets: updatedBuckets });
    setEditingIndex(null);
    setEditBucketCors('');
  };

  const handleDeleteBucket = (index: number) => {
    const updatedBuckets = buckets.filter((_, i) => i !== index);
    onConfigChange({ buckets: updatedBuckets });
  };

  return (
    <div className="space-y-4">
      {/* Static Backend Bucket */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HardDrive className="w-5 h-5" />
            Primary Backend Bucket
          </CardTitle>
          <CardDescription>Main S3 bucket for backend file storage</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div className="flex items-center justify-between p-3 bg-gray-800 rounded">
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <HardDrive className="w-4 h-4 text-green-400" />
                  <code className="text-sm font-mono text-green-400">
                    {config.project}-backend-{config.env}{config.workload?.bucket_postfix || ''}
                  </code>
                </div>
                <div className="flex items-center gap-4 mt-2">
                  <div className="flex items-center gap-2">
                    {config.workload?.bucket_public ? (
                      <Globe className="w-3 h-3 text-yellow-400" />
                    ) : (
                      <Lock className="w-3 h-3 text-gray-400" />
                    )}
                    <Badge variant={config.workload?.bucket_public ? "warning" : "secondary"} className="text-xs">
                      {config.workload?.bucket_public ? 'Public Access' : 'Private'}
                    </Badge>
                  </div>
                  <Badge variant="outline" className="text-xs">Primary</Badge>
                </div>
              </div>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => window.open(
                  `https://s3.console.aws.amazon.com/s3/buckets/${config.project}-backend-${config.env}${config.workload?.bucket_postfix || ''}?region=${config.region}`,
                  '_blank'
                )}
              >
                <ExternalLink className="w-4 h-4" />
              </Button>
            </div>

            <div className="space-y-2 text-sm text-gray-400">
              <p className="flex items-center gap-2">
                <span className="text-gray-500">Environment Variable:</span>
                <code className="font-mono text-blue-400">AWS_S3_BUCKET</code>
              </p>
              <p className="flex items-center gap-2">
                <span className="text-gray-500">Purpose:</span>
                <span>File uploads, static assets, temporary storage</span>
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Additional Buckets */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <HardDrive className="w-5 h-5" />
                Additional Buckets
              </CardTitle>
              <CardDescription>
                Configure additional S3 buckets for your application
              </CardDescription>
            </div>
            <Button
              size="sm"
              onClick={() => setShowNewBucket(true)}
              disabled={showNewBucket}
            >
              <Plus className="w-4 h-4 mr-1" />
              Add Bucket
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* New bucket form */}
          {showNewBucket && (
            <div className="border border-blue-700 bg-blue-900/10 rounded-lg p-4 space-y-4">
              <div className="space-y-3">
                <div>
                  <Label htmlFor="bucket-name">Bucket Name Suffix</Label>
                  <Input
                    id="bucket-name"
                    value={newBucketName}
                    onChange={(e) => setNewBucketName(e.target.value)}
                    placeholder="uploads"
                    className="mt-1"
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    Final name: {config.project}-{newBucketName || 'name'}-{config.env}
                  </p>
                </div>

                <div className="space-y-3">
                  <div className="flex items-center justify-between p-3 bg-gray-900 rounded-lg">
                    <div className="space-y-1">
                      <Label htmlFor="bucket-public" className="text-sm font-medium">Public Access</Label>
                      <p className="text-xs text-gray-400">Allow unauthenticated access to objects</p>
                    </div>
                    <Switch
                      id="bucket-public"
                      checked={newBucketPublic}
                      onCheckedChange={(checked) => {
                        setNewBucketPublic(checked);
                        // If enabling public and no CORS is set, add default
                        if (checked && !newBucketCors.trim()) {
                          setNewBucketCors(JSON.stringify(defaultPublicCors, null, 2));
                        }
                      }}
                    />
                  </div>
                  
                  <div className="flex items-center justify-between p-3 bg-gray-900 rounded-lg">
                    <div className="space-y-1">
                      <Label htmlFor="bucket-versioning" className="text-sm font-medium">Versioning</Label>
                      <p className="text-xs text-gray-400">Keep history of object changes</p>
                    </div>
                    <Switch
                      id="bucket-versioning"
                      checked={newBucketVersioning}
                      onCheckedChange={setNewBucketVersioning}
                    />
                  </div>
                </div>

                <div>
                  <Label htmlFor="bucket-cors">CORS Rules (JSON, optional)</Label>
                  <Textarea
                    id="bucket-cors"
                    value={newBucketCors}
                    onChange={(e) => setNewBucketCors(e.target.value)}
                    placeholder={newBucketPublic ? 'Default CORS for public access will be applied' : `{
  "allowed_origins": ["https://app.example.com"],
  "allowed_methods": ["GET", "PUT"],
  "max_age_seconds": 7200
}`}
                    className="mt-1 font-mono text-xs h-24"
                  />
                  {newBucketPublic && !newBucketCors.trim() && (
                    <p className="text-xs text-blue-400 mt-1">
                      Default CORS will be applied: GET/HEAD from all origins
                    </p>
                  )}
                </div>
              </div>

              <div className="flex justify-end gap-2">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setShowNewBucket(false);
                    setNewBucketName('');
                    setNewBucketPublic(false);
                    setNewBucketVersioning(true);
                    setNewBucketCors('');
                  }}
                >
                  Cancel
                </Button>
                <Button
                  size="sm"
                  onClick={handleAddBucket}
                  disabled={!newBucketName}
                >
                  <Check className="w-4 h-4 mr-1" />
                  Add Bucket
                </Button>
              </div>
            </div>
          )}

          {/* Existing buckets */}
          {buckets.map((bucket, index) => {
            const isEditing = editingIndex === index;
            const fullBucketName = `${config.project}-${bucket.name}-${config.env}`;

            return (
              <div key={index} className="border border-gray-700 rounded-lg p-3">
                {isEditing ? (
                  <div className="space-y-4">
                    <div>
                      <Label className="text-xs text-gray-400 mb-1">Bucket Name Suffix</Label>
                      <Input
                        value={editBucket.name}
                        onChange={(e) => setEditBucket({ ...editBucket, name: e.target.value })}
                        className="font-mono text-sm"
                      />
                      <p className="text-xs text-gray-500 mt-1">
                        Full name: {config.project}-{editBucket.name || 'name'}-{config.env}
                      </p>
                    </div>
                    
                    <div className="space-y-3">
                      <div className="flex items-center justify-between p-3 bg-gray-900 rounded-lg">
                        <div className="space-y-1">
                          <Label className="text-sm font-medium">Public Access</Label>
                          <p className="text-xs text-gray-400">Allow unauthenticated access to objects</p>
                        </div>
                        <Switch
                          checked={editBucket.public || false}
                          onCheckedChange={(checked) => {
                            setEditBucket({ ...editBucket, public: checked });
                            // Add default CORS for public buckets
                            if (checked && !editBucketCors) {
                              setEditBucketCors(JSON.stringify(defaultPublicCors, null, 2));
                            }
                          }}
                        />
                      </div>
                      
                      <div className="flex items-center justify-between p-3 bg-gray-900 rounded-lg">
                        <div className="space-y-1">
                          <Label className="text-sm font-medium">Versioning</Label>
                          <p className="text-xs text-gray-400">Keep history of object changes</p>
                        </div>
                        <Switch
                          checked={editBucket.versioning !== false}
                          onCheckedChange={(checked) => setEditBucket({ ...editBucket, versioning: checked })}
                        />
                      </div>
                    </div>
                    
                    <div>
                      <Label className="text-xs text-gray-400 mb-1">CORS Configuration (JSON)</Label>
                      <Textarea
                        value={editBucketCors}
                        onChange={(e) => setEditBucketCors(e.target.value)}
                        placeholder={editBucket.public ? 'Default CORS for public access will be applied if empty' : 'Optional CORS rules in JSON format'}
                        className="font-mono text-xs h-32"
                      />
                      {editBucket.public && !editBucketCors.trim() && (
                        <p className="text-xs text-blue-400 mt-1">
                          Default: GET/HEAD from all origins
                        </p>
                      )}
                    </div>
                    
                    <div className="flex justify-end gap-2 pt-2">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          setEditingIndex(null);
                          setEditBucketCors('');
                        }}
                      >
                        Cancel
                      </Button>
                      <Button
                        size="sm"
                        onClick={() => handleUpdateBucket(index)}
                      >
                        <Check className="w-4 h-4 mr-1" />
                        Save Changes
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center justify-between">
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <HardDrive className="w-4 h-4 text-blue-400" />
                        <code className="text-sm font-mono text-blue-400">{fullBucketName}</code>
                      </div>
                      <div className="flex items-center gap-3 text-xs">
                        <div className="flex items-center gap-1">
                          {bucket.public ? (
                            <Globe className="w-3 h-3 text-yellow-400" />
                          ) : (
                            <Lock className="w-3 h-3 text-green-400" />
                          )}
                          <span className="text-gray-400">
                            {bucket.public ? 'Public' : 'Private'}
                          </span>
                        </div>
                        <div className="flex items-center gap-1">
                          <History className="w-3 h-3 text-blue-400" />
                          <span className="text-gray-400">
                            Versioning {bucket.versioning === false ? 'Off' : 'On'}
                          </span>
                        </div>
                        {bucket.cors_rules && (
                          <div className="flex items-center gap-1">
                            <Shield className="w-3 h-3 text-purple-400" />
                            <span className="text-gray-400">CORS</span>
                          </div>
                        )}
                      </div>
                    </div>
                    <div className="flex items-center gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => window.open(
                          `https://s3.console.aws.amazon.com/s3/buckets/${fullBucketName}?region=${config.region}`,
                          '_blank'
                        )}
                        className="h-7 w-7 p-0"
                      >
                        <ExternalLink className="w-3 h-3" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          setEditingIndex(index);
                          setEditBucket(bucket);
                          // Set CORS field if bucket has CORS rules
                          if (bucket.cors_rules && bucket.cors_rules.length > 0) {
                            setEditBucketCors(JSON.stringify(
                              bucket.cors_rules.length === 1 ? bucket.cors_rules[0] : bucket.cors_rules,
                              null,
                              2
                            ));
                          } else {
                            setEditBucketCors('');
                          }
                        }}
                        className="h-7 w-7 p-0"
                      >
                        <Edit2 className="w-3 h-3" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleDeleteBucket(index)}
                        className="h-7 w-7 p-0 text-red-400 hover:text-red-300"
                      >
                        <Trash2 className="w-3 h-3" />
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            );
          })}

          {buckets.length === 0 && !showNewBucket && (
            <div className="text-center py-8 text-gray-400">
              <HardDrive className="w-8 h-8 mx-auto mb-2 opacity-50" />
              <p className="text-sm">No additional buckets configured</p>
              <p className="text-xs mt-1">Click "Add Bucket" to create a new S3 bucket</p>
            </div>
          )}
        </CardContent>
      </Card>

    </div>
  );
}
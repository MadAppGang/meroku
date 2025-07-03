import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { HardDrive, Globe, Lock, Folder } from 'lucide-react';
import { YamlInfrastructureConfig } from '../types/yamlConfig';

interface BackendS3BucketsProps {
  config: YamlInfrastructureConfig;
}

export function BackendS3Buckets({ config }: BackendS3BucketsProps) {
  const primaryBucketName = `${config.project}-backend-${config.env}-${config.workload?.bucket_postfix || ''}`;
  const isPublic = config.workload?.bucket_public !== false;

  // Additional buckets from config
  const additionalBuckets = config.buckets || [];

  return (
    <div className="space-y-4">
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
                  <Folder className="w-4 h-4 text-green-400" />
                  <code className="text-sm font-mono text-green-400">{primaryBucketName}</code>
                </div>
                <div className="flex items-center gap-4 mt-2">
                  <div className="flex items-center gap-2">
                    {isPublic ? <Globe className="w-3 h-3 text-yellow-400" /> : <Lock className="w-3 h-3 text-gray-400" />}
                    <Badge variant={isPublic ? "warning" : "secondary"} className="text-xs">
                      {isPublic ? 'Public Access' : 'Private'}
                    </Badge>
                  </div>
                  <Badge variant="outline" className="text-xs">Primary</Badge>
                </div>
              </div>
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

            <div className="p-3 bg-gray-800 rounded space-y-2">
              <p className="text-xs font-medium text-gray-300">IAM Permissions:</p>
              <ul className="text-xs text-gray-400 space-y-1 ml-4">
                <li>• s3:GetObject</li>
                <li>• s3:PutObject</li>
                <li>• s3:DeleteObject</li>
                <li>• s3:ListBucket</li>
              </ul>
            </div>
          </div>
        </CardContent>
      </Card>

      {additionalBuckets.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Additional Buckets</CardTitle>
            <CardDescription>Extra S3 buckets configured in YAML</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {additionalBuckets.map((bucket, index) => (
                <div key={index} className="flex items-center justify-between p-3 bg-gray-800 rounded">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <Folder className="w-4 h-4 text-blue-400" />
                      <code className="text-sm font-mono text-blue-400">{bucket.name}</code>
                    </div>
                    <p className="text-xs text-gray-400">{bucket.description || 'Additional storage bucket'}</p>
                  </div>
                  <Badge variant="outline" className="text-xs">Additional</Badge>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>S3 Configuration</CardTitle>
          <CardDescription>Bucket settings and features</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3 text-sm">
              <div className="p-2 bg-gray-800 rounded">
                <p className="text-gray-400 text-xs">Versioning</p>
                <p className="text-gray-200">Enabled</p>
              </div>
              <div className="p-2 bg-gray-800 rounded">
                <p className="text-gray-400 text-xs">Encryption</p>
                <p className="text-gray-200">AES256</p>
              </div>
              <div className="p-2 bg-gray-800 rounded">
                <p className="text-gray-400 text-xs">Region</p>
                <p className="text-gray-200">Current Region</p>
              </div>
              <div className="p-2 bg-gray-800 rounded">
                <p className="text-gray-400 text-xs">Lifecycle</p>
                <p className="text-gray-200">Not configured</p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
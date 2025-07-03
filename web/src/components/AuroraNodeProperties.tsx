import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Database, Info } from 'lucide-react';

export function AuroraNodeProperties() {
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Amazon Aurora</CardTitle>
          <CardDescription>
            High-performance managed relational database
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="bg-gray-800 border border-gray-600 rounded-lg p-6 text-center space-y-4">
            <Database className="w-12 h-12 text-gray-500 mx-auto" />
            <h3 className="text-lg font-medium text-gray-300">Not Implemented</h3>
            <p className="text-sm text-gray-400">
              Amazon Aurora is not implemented in this infrastructure template.
            </p>
          </div>

          <div className="mt-6 bg-blue-900/20 border border-blue-700 rounded-lg p-4">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-blue-400 mt-0.5" />
              <div className="flex-1">
                <h4 className="text-sm font-medium text-blue-400 mb-2">Alternative Solution</h4>
                <p className="text-xs text-gray-300">
                  This infrastructure uses <strong>RDS PostgreSQL</strong> instead of Aurora. 
                  RDS PostgreSQL provides a managed PostgreSQL database with automated backups, 
                  high availability, and read replicas support.
                </p>
                <p className="text-xs text-gray-400 mt-2">
                  To configure the database, look for the PostgreSQL node in the infrastructure diagram.
                </p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
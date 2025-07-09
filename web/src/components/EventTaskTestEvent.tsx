import { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Button } from './ui/button';
import { Alert, AlertDescription } from './ui/alert';
import { Textarea } from './ui/textarea';
import { YamlInfrastructureConfig } from '../types/yamlConfig';
import { infrastructureApi, type TestEventRequest, type TestEventResponse } from '../api/infrastructure';
import { ComponentNode } from '../types';
import { 
  Send,
  Clock,
  CheckCircle,
  XCircle,
  Info,
  Zap
} from 'lucide-react';

interface EventTaskTestEventProps {
  config: YamlInfrastructureConfig;
  node: ComponentNode;
}

export function EventTaskTestEvent({ config, node }: EventTaskTestEventProps) {
  // Extract task name from node id
  const taskName = node.id.replace('event-', '');
  
  // Find the event task configuration
  const eventTask = config.event_processor_tasks?.find(task => task.name === taskName);
  
  // Test event state - default source to 'meroku.test'
  const [testEvent, setTestEvent] = useState<TestEventRequest>({
    source: 'meroku.test',
    detailType: eventTask?.detail_types?.[0] || '',
    detail: {}
  });
  const [detailJson, setDetailJson] = useState('{\n  "test": true,\n  "timestamp": "' + new Date().toISOString() + '"\n}');
  const [sendingEvent, setSendingEvent] = useState(false);
  const [eventResponse, setEventResponse] = useState<TestEventResponse | null>(null);
  const [jsonError, setJsonError] = useState<string | null>(null);
  
  // Initialize test event when task changes
  useEffect(() => {
    if (eventTask) {
      setTestEvent(prev => ({
        ...prev,
        detailType: eventTask.detail_types?.[0] || prev.detailType
      }));
    }
  }, [eventTask]);

  const handleDetailJsonChange = (value: string) => {
    setDetailJson(value);
    try {
      const parsed = JSON.parse(value);
      setTestEvent(prev => ({ ...prev, detail: parsed }));
      setJsonError(null);
    } catch (e) {
      setJsonError('Invalid JSON format');
    }
  };

  const handleSendTestEvent = async () => {
    if (!testEvent.source || !testEvent.detailType) {
      setEventResponse({
        success: false,
        message: 'Source and Detail Type are required'
      });
      return;
    }

    setSendingEvent(true);
    setEventResponse(null);

    try {
      const response = await infrastructureApi.sendTestEvent(testEvent);
      setEventResponse(response);
    } catch (error) {
      setEventResponse({
        success: false,
        message: error instanceof Error ? error.message : 'Failed to send test event'
      });
    } finally {
      setSendingEvent(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* Info Alert */}
      <Alert>
        <Info className="h-4 w-4" />
        <AlertDescription>
          Send test events to EventBridge to trigger this task. The event must match the configured 
          sources and detail types for the rule to trigger.
        </AlertDescription>
      </Alert>

      {/* Test Event Configuration */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Send className="w-5 h-5" />
            Test Event Configuration
          </CardTitle>
          <CardDescription>
            Configure and send a test event to EventBridge
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="test-source">Event Source</Label>
            <Input
              id="test-source"
              value={testEvent.source}
              onChange={(e) => setTestEvent(prev => ({ ...prev, source: e.target.value }))}
              placeholder="e.g., meroku.test"
              className="font-mono"
            />
            {eventTask?.sources && eventTask.sources.length > 0 && (
              <div className="space-y-1">
                <p className="text-xs text-gray-500">
                  Configured sources: {eventTask.sources.join(', ')}
                </p>
                {!eventTask.sources.includes(testEvent.source) && testEvent.source && (
                  <p className="text-xs text-yellow-500 flex items-center gap-1">
                    <Info className="w-3 h-3" />
                    This source doesn't match any configured sources - the event won't trigger this task
                  </p>
                )}
              </div>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="test-detail-type">Detail Type</Label>
            <Input
              id="test-detail-type"
              value={testEvent.detailType}
              onChange={(e) => setTestEvent(prev => ({ ...prev, detailType: e.target.value }))}
              placeholder="e.g., Test Event"
            />
            {eventTask?.detail_types && eventTask.detail_types.length > 0 && (
              <div className="space-y-1">
                <p className="text-xs text-gray-500">
                  Configured detail types: {eventTask.detail_types.join(', ')}
                </p>
                {!eventTask.detail_types.includes(testEvent.detailType) && testEvent.detailType && (
                  <p className="text-xs text-yellow-500 flex items-center gap-1">
                    <Info className="w-3 h-3" />
                    This detail type doesn't match any configured types - the event won't trigger this task
                  </p>
                )}
              </div>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="test-detail">Event Detail (JSON)</Label>
            <Textarea
              id="test-detail"
              value={detailJson}
              onChange={(e) => handleDetailJsonChange(e.target.value)}
              placeholder='{"orderId": "123", "amount": 99.99}'
              className="font-mono text-sm min-h-[200px]"
              rows={10}
            />
            {jsonError && (
              <p className="text-xs text-red-400">{jsonError}</p>
            )}
          </div>

          <Button 
            onClick={handleSendTestEvent} 
            disabled={sendingEvent || !!jsonError || !testEvent.source || !testEvent.detailType}
            className="w-full"
          >
            {sendingEvent ? (
              <>
                <Clock className="w-4 h-4 mr-2 animate-spin" />
                Sending...
              </>
            ) : (
              <>
                <Send className="w-4 h-4 mr-2" />
                Send Test Event
              </>
            )}
          </Button>

          {eventResponse && (
            <Alert className={eventResponse.success ? 'border-green-600' : 'border-red-600'}>
              {eventResponse.success ? (
                <CheckCircle className="h-4 w-4 text-green-600" />
              ) : (
                <XCircle className="h-4 w-4 text-red-600" />
              )}
              <AlertDescription>
                {eventResponse.message}
                {eventResponse.eventId && (
                  <div className="text-xs mt-1 font-mono">Event ID: {eventResponse.eventId}</div>
                )}
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      {/* Example Events */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Zap className="w-5 h-5" />
            Example Test Events
          </CardTitle>
          <CardDescription>
            Common test event patterns for different scenarios
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-3">
            <div className="p-3 bg-gray-800 rounded-lg">
              <h4 className="text-sm font-medium text-gray-300 mb-2">Basic Test Event</h4>
              <pre className="text-xs text-gray-400 overflow-x-auto">{`{
  "test": true,
  "timestamp": "${new Date().toISOString()}",
  "message": "This is a test event"
}`}</pre>
            </div>

            <div className="p-3 bg-gray-800 rounded-lg">
              <h4 className="text-sm font-medium text-gray-300 mb-2">Order Processing Event</h4>
              <pre className="text-xs text-gray-400 overflow-x-auto">{`{
  "orderId": "ORD-123456",
  "customerId": "CUST-789",
  "amount": 99.99,
  "currency": "USD",
  "items": [
    {
      "sku": "PROD-001",
      "quantity": 2,
      "price": 49.99
    }
  ]
}`}</pre>
            </div>

            <div className="p-3 bg-gray-800 rounded-lg">
              <h4 className="text-sm font-medium text-gray-300 mb-2">User Action Event</h4>
              <pre className="text-xs text-gray-400 overflow-x-auto">{`{
  "userId": "user-123",
  "action": "profile_updated",
  "changes": {
    "email": "new@example.com",
    "name": "John Doe"
  },
  "timestamp": "${new Date().toISOString()}"
}`}</pre>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Testing Tips */}
      <Card>
        <CardHeader>
          <CardTitle>Testing Tips</CardTitle>
          <CardDescription>
            Best practices for testing event-driven tasks
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ul className="text-sm text-gray-300 space-y-2">
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Use <code className="text-blue-400 text-xs">meroku.test</code> as the source for test events to easily identify them in logs</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Include a <code className="text-blue-400 text-xs">test: true</code> field in your event detail for easy filtering</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Always include timestamps in your test events for debugging</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Check CloudWatch Logs immediately after sending to see task execution</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-blue-400">•</span>
              <span>Events that don't match the configured pattern won't trigger the task</span>
            </li>
          </ul>
        </CardContent>
      </Card>
    </div>
  );
}
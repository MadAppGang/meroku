import { useState, useEffect, useRef, useCallback } from 'react';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { Input } from './ui/input';
import { Switch } from './ui/switch';
import { Label } from './ui/label';
import { ScrollArea } from './ui/scroll-area';
import { Alert, AlertDescription } from './ui/alert';
import { 
  RefreshCw, 
  Download, 
  Search, 
  Pause, 
  Play, 
  Trash2,
  Filter,
  AlertCircle,
  Wifi,
  Maximize,
  X
} from 'lucide-react';
import { infrastructureApi, LogEntry } from '../api/infrastructure';
import { format, parseISO } from 'date-fns';

interface ServiceLogsProps {
  environment: string;
  serviceName: string;
}

export function ServiceLogs({ environment, serviceName }: ServiceLogsProps) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterLevel, setFilterLevel] = useState<string>('all');
  const [autoScroll, setAutoScroll] = useState(true);
  const [isStreaming, setIsStreaming] = useState(false);
  const [nextToken, setNextToken] = useState<string | undefined>();
  const [isFullScreen, setIsFullScreen] = useState(false);
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);

  // Format timestamp for display
  const formatTimestamp = (timestamp: string) => {
    try {
      return format(parseISO(timestamp), 'MMM dd HH:mm:ss.SSS');
    } catch {
      return timestamp;
    }
  };

  // Load initial logs
  const loadLogs = useCallback(async (append: boolean = false) => {
    try {
      setLoading(true);
      setError(null);
      const response = await infrastructureApi.getServiceLogs(
        environment,
        serviceName,
        100,
        append ? nextToken : undefined
      );
      
      setLogs(prev => append ? [...prev, ...response.logs] : response.logs);
      setNextToken(response.nextToken);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch logs');
    } finally {
      setLoading(false);
    }
  }, [environment, serviceName, nextToken]);

  // Start/stop log streaming
  const toggleStreaming = useCallback(() => {
    if (isStreaming && wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
      setIsStreaming(false);
    } else {
      setIsStreaming(true);
      // WebSocket will show logs from last 24 hours, then stream new ones
      wsRef.current = infrastructureApi.connectToLogStream(
        environment,
        serviceName,
        (newLogs) => {
          setLogs(prev => [...newLogs, ...prev].slice(0, 1000)); // Keep max 1000 logs
          if (autoScroll && scrollAreaRef.current) {
            scrollAreaRef.current.scrollTop = 0;
          }
        },
        (error) => {
          console.error('WebSocket error:', error);
          setError('Lost connection to log stream');
          setIsStreaming(false);
        },
        () => {
          setError(null);
        }
      );
    }
  }, [environment, serviceName, isStreaming, autoScroll]);

  // Initial load
  useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  // Cleanup WebSocket on unmount
  useEffect(() => {
    return () => {
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, []);

  // Filter logs
  const filteredLogs = logs.filter(log => {
    const matchesSearch = searchTerm === '' || 
      log.message.toLowerCase().includes(searchTerm.toLowerCase()) ||
      log.stream.toLowerCase().includes(searchTerm.toLowerCase());
    
    const matchesLevel = filterLevel === 'all' || log.level === filterLevel;
    
    return matchesSearch && matchesLevel;
  });

  // Export logs
  const exportLogs = () => {
    const content = filteredLogs.map(log => 
      `[${log.timestamp}] [${log.level.toUpperCase()}] [${log.stream}] ${log.message}`
    ).join('\n');
    
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${serviceName}-logs-${new Date().toISOString()}.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  // Get level badge color
  const getLevelBadgeVariant = (level: string) => {
    switch (level) {
      case 'error': return 'destructive';
      case 'warning': return 'outline';
      case 'debug': return 'secondary';
      default: return 'default';
    }
  };

  return (
    <div className="flex flex-col h-full space-y-4">
      {/* Controls */}
      <div className="space-y-4">
        {/* Top controls */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant={isStreaming ? "destructive" : "default"}
              onClick={toggleStreaming}
              title={isStreaming ? "Stop real-time log streaming" : "Stream logs from last 24 hours + new logs"}
            >
              {isStreaming ? (
                <>
                  <Pause className="w-4 h-4 mr-2" />
                  Stop Streaming
                </>
              ) : (
                <>
                  <Play className="w-4 h-4 mr-2" />
                  Start Streaming
                </>
              )}
            </Button>
            
            <Button
              size="sm"
              variant="outline"
              onClick={() => loadLogs(false)}
              disabled={loading || isStreaming}
              title="Fetch latest 100 logs across all time"
            >
              <RefreshCw className={`w-4 h-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>

            {isStreaming && (
              <Badge variant="default" className="animate-pulse">
                <Wifi className="w-3 h-3 mr-1" />
                Live
              </Badge>
            )}
          </div>

          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => setIsFullScreen(true)}
            >
              <Maximize className="w-4 h-4 mr-2" />
              Full Screen
            </Button>
            
            <Button
              size="sm"
              variant="outline"
              onClick={() => setLogs([])}
            >
              <Trash2 className="w-4 h-4 mr-2" />
              Clear
            </Button>
            
            <Button
              size="sm"
              variant="outline"
              onClick={exportLogs}
              disabled={filteredLogs.length === 0}
            >
              <Download className="w-4 h-4 mr-2" />
              Export
            </Button>
          </div>
        </div>

        {/* Filters */}
        <div className="flex items-center gap-4">
          <div className="flex-1 relative">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
            <Input
              placeholder="Search logs..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="pl-10 bg-gray-800 border-gray-600"
            />
          </div>

          <div className="flex items-center gap-2">
            <Filter className="w-4 h-4 text-gray-400" />
            <select
              value={filterLevel}
              onChange={(e) => setFilterLevel(e.target.value)}
              className="px-3 py-1 bg-gray-800 border border-gray-600 rounded-md text-sm text-white"
            >
              <option value="all">All Levels</option>
              <option value="error">Error</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
              <option value="debug">Debug</option>
            </select>
          </div>

          <div className="flex items-center gap-2">
            <Switch
              id="auto-scroll"
              checked={autoScroll}
              onCheckedChange={setAutoScroll}
            />
            <Label htmlFor="auto-scroll" className="text-sm">
              Auto-scroll
            </Label>
          </div>
        </div>

        {/* Stats */}
        <div className="flex items-center gap-4 text-sm text-gray-400">
          <span>Total: {logs.length}</span>
          <span>Filtered: {filteredLogs.length}</span>
          <span>
            Errors: {logs.filter(l => l.level === 'error').length}
          </span>
          <span>
            Warnings: {logs.filter(l => l.level === 'warning').length}
          </span>
        </div>
      </div>

      {/* Error Alert */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {/* Logs Display */}
      <div className="flex-1 min-h-0 bg-gray-900 rounded-lg border border-gray-700">
        <ScrollArea className="h-full" ref={scrollAreaRef}>
          <div className="p-4 space-y-1">
            {loading && logs.length === 0 ? (
              <div className="flex items-center justify-center h-32">
                <RefreshCw className="w-8 h-8 animate-spin text-gray-400" />
              </div>
            ) : filteredLogs.length === 0 ? (
              <div className="text-center text-gray-500 py-8">
                {searchTerm || filterLevel !== 'all' 
                  ? 'No logs match your filters' 
                  : 'No logs available'}
                {!isStreaming && logs.length === 0 && !searchTerm && filterLevel === 'all' && (
                  <div className="mt-4 space-y-2">
                    <p className="text-xs">Try one of these options:</p>
                    <p className="text-xs">• Click "Start Streaming" to see logs from the last 24 hours</p>
                    <p className="text-xs">• Check if the service is running and generating logs</p>
                  </div>
                )}
              </div>
            ) : (
              filteredLogs.map((log, index) => (
                <div
                  key={`${log.timestamp}-${index}`}
                  className={`
                    font-mono text-xs p-2 rounded border-l-4 
                    ${log.level === 'error' ? 'bg-red-900/20 border-red-500' :
                      log.level === 'warning' ? 'bg-yellow-900/20 border-yellow-500' :
                      log.level === 'debug' ? 'bg-gray-800 border-gray-600' :
                      'bg-gray-800 border-blue-500'}
                    hover:bg-gray-700/50 transition-colors
                  `}
                >
                  <div className="flex items-start gap-2">
                    <span className="text-gray-500 whitespace-nowrap">
                      {formatTimestamp(log.timestamp)}
                    </span>
                    <Badge 
                      variant={getLevelBadgeVariant(log.level)}
                      className="text-xs px-1 py-0"
                    >
                      {log.level.toUpperCase()}
                    </Badge>
                    <span className="text-gray-500 text-xs truncate max-w-[200px]" title={log.stream}>
                      [{log.stream}]
                    </span>
                  </div>
                  <div className="mt-1 text-gray-300 whitespace-pre-wrap break-all">
                    {log.message}
                  </div>
                </div>
              ))
            )}
            
            {/* Load more button */}
            {!loading && nextToken && !isStreaming && (
              <div className="text-center pt-4">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => loadLogs(true)}
                >
                  Load More
                </Button>
              </div>
            )}
          </div>
        </ScrollArea>
      </div>

      {/* Full Screen Modal */}
      {isFullScreen && (
        <div className="fixed inset-0 z-50 bg-gray-950 flex flex-col">
          {/* Full Screen Header */}
          <div className="flex items-center justify-between p-4 border-b border-gray-700 bg-gray-900">
            <div className="flex items-center gap-4">
              <h2 className="text-lg font-semibold text-white">
                {serviceName} Logs - {environment}
              </h2>
              {isStreaming && (
                <Badge variant="default" className="animate-pulse">
                  <Wifi className="w-3 h-3 mr-1" />
                  Live
                </Badge>
              )}
            </div>
            
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant={isStreaming ? "destructive" : "default"}
                onClick={toggleStreaming}
                title={isStreaming ? "Stop real-time log streaming" : "Stream logs from last 24 hours + new logs"}
              >
                {isStreaming ? (
                  <>
                    <Pause className="w-4 h-4 mr-2" />
                    Stop Streaming
                  </>
                ) : (
                  <>
                    <Play className="w-4 h-4 mr-2" />
                    Start Streaming
                  </>
                )}
              </Button>
              
              <Button
                size="sm"
                variant="outline"
                onClick={() => loadLogs(false)}
                disabled={loading || isStreaming}
                title="Fetch latest 100 logs across all time"
              >
                <RefreshCw className={`w-4 h-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
                Refresh
              </Button>

              <Button
                size="sm"
                variant="outline"
                onClick={() => setLogs([])}
              >
                <Trash2 className="w-4 h-4 mr-2" />
                Clear
              </Button>
              
              <Button
                size="sm"
                variant="outline"
                onClick={exportLogs}
                disabled={filteredLogs.length === 0}
              >
                <Download className="w-4 h-4 mr-2" />
                Export
              </Button>

              <Button
                size="sm"
                variant="ghost"
                onClick={() => setIsFullScreen(false)}
              >
                <X className="w-4 h-4" />
              </Button>
            </div>
          </div>

          {/* Full Screen Filters */}
          <div className="p-4 border-b border-gray-700 bg-gray-900">
            <div className="flex items-center gap-4">
              <div className="flex-1 relative">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
                <Input
                  placeholder="Search logs..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-10 bg-gray-800 border-gray-600"
                />
              </div>

              <div className="flex items-center gap-2">
                <Filter className="w-4 h-4 text-gray-400" />
                <select
                  value={filterLevel}
                  onChange={(e) => setFilterLevel(e.target.value)}
                  className="px-3 py-1 bg-gray-800 border border-gray-600 rounded-md text-sm text-white"
                >
                  <option value="all">All Levels</option>
                  <option value="error">Error</option>
                  <option value="warning">Warning</option>
                  <option value="info">Info</option>
                  <option value="debug">Debug</option>
                </select>
              </div>

              <div className="flex items-center gap-2">
                <Switch
                  id="auto-scroll-fullscreen"
                  checked={autoScroll}
                  onCheckedChange={setAutoScroll}
                />
                <Label htmlFor="auto-scroll-fullscreen" className="text-sm">
                  Auto-scroll
                </Label>
              </div>

              <div className="flex items-center gap-4 text-sm text-gray-400">
                <span>Total: {logs.length}</span>
                <span>Filtered: {filteredLogs.length}</span>
                <span>Errors: {logs.filter(l => l.level === 'error').length}</span>
                <span>Warnings: {logs.filter(l => l.level === 'warning').length}</span>
              </div>
            </div>
          </div>

          {/* Error Alert in Full Screen */}
          {error && (
            <div className="p-4 bg-gray-900 border-b border-gray-700">
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            </div>
          )}

          {/* Full Screen Logs Display */}
          <div className="flex-1 min-h-0 bg-gray-950 overflow-hidden">
            <ScrollArea className="h-full" ref={scrollAreaRef}>
              <div className="p-6 space-y-2">
                {loading && logs.length === 0 ? (
                  <div className="flex items-center justify-center h-64">
                    <RefreshCw className="w-12 h-12 animate-spin text-gray-400" />
                  </div>
                ) : filteredLogs.length === 0 ? (
                  <div className="text-center text-gray-500 py-32 text-lg">
                    {searchTerm || filterLevel !== 'all' 
                      ? 'No logs match your filters' 
                      : 'No logs available'}
                  </div>
                ) : (
                  filteredLogs.map((log, index) => (
                    <div
                      key={`${log.timestamp}-${index}`}
                      className={`
                        font-mono text-sm p-4 rounded-lg border-l-4 
                        ${log.level === 'error' ? 'bg-red-900/20 border-red-500' :
                          log.level === 'warning' ? 'bg-yellow-900/20 border-yellow-500' :
                          log.level === 'debug' ? 'bg-gray-800 border-gray-600' :
                          'bg-gray-800 border-blue-500'}
                        hover:bg-gray-700/50 transition-colors
                      `}
                    >
                      <div className="flex items-start gap-3 mb-2">
                        <span className="text-gray-400 whitespace-nowrap font-medium">
                          {formatTimestamp(log.timestamp)}
                        </span>
                        <Badge 
                          variant={getLevelBadgeVariant(log.level)}
                          className="text-xs px-2 py-1"
                        >
                          {log.level.toUpperCase()}
                        </Badge>
                        <span className="text-gray-400 text-sm truncate max-w-[300px]" title={log.stream}>
                          [{log.stream}]
                        </span>
                      </div>
                      <div className="text-gray-200 whitespace-pre-wrap break-all leading-relaxed">
                        {log.message}
                      </div>
                    </div>
                  ))
                )}
                
                {/* Load more button in full screen */}
                {!loading && nextToken && !isStreaming && (
                  <div className="text-center pt-6">
                    <Button
                      size="default"
                      variant="outline"
                      onClick={() => loadLogs(true)}
                    >
                      Load More Logs
                    </Button>
                  </div>
                )}
              </div>
            </ScrollArea>
          </div>
        </div>
      )}
    </div>
  );
}
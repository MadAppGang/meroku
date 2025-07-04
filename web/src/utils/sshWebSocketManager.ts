import { type SSHMessage } from '../api/infrastructure';

interface ConnectionConfig {
  url: string;
  onMessage: (msg: SSHMessage) => void;
  onError?: (error: Error) => void;
  onClose?: () => void;
  onOpen?: () => void;
}

export class SSHWebSocketManager {
  private static instance: SSHWebSocketManager;
  private connections: Map<string, WebSocket> = new Map();
  private configs: Map<string, ConnectionConfig> = new Map();

  static getInstance(): SSHWebSocketManager {
    if (!SSHWebSocketManager.instance) {
      SSHWebSocketManager.instance = new SSHWebSocketManager();
    }
    return SSHWebSocketManager.instance;
  }

  getConnection(key: string, config: ConnectionConfig): WebSocket {
    // Return existing connection if available and open
    const existing = this.connections.get(key);
    if (existing && (existing.readyState === WebSocket.OPEN || existing.readyState === WebSocket.CONNECTING)) {
      console.log(`Reusing existing connection for key: ${key}, state: ${existing.readyState}`);
      // Update callbacks
      this.configs.set(key, config);
      
      // If already open, call onOpen immediately
      if (existing.readyState === WebSocket.OPEN) {
        config.onOpen?.();
      }
      
      return existing;
    }

    // Create new connection
    console.log(`Creating new WebSocket connection for key: ${key}, URL: ${config.url}`);
    const ws = new WebSocket(config.url);
    this.connections.set(key, ws);
    this.configs.set(key, config);

    ws.onopen = () => {
      console.log(`SSH WebSocket connected for key: ${key}`);
      config.onOpen?.();
    };

    ws.onmessage = (event) => {
      try {
        const message: SSHMessage = JSON.parse(event.data);
        // Get the latest config in case it was updated
        const currentConfig = this.configs.get(key);
        currentConfig?.onMessage(message);
      } catch (err) {
        console.error('Failed to parse SSH message:', err);
      }
    };

    ws.onerror = (event) => {
      console.error(`SSH WebSocket error for key ${key}:`, event);
      const currentConfig = this.configs.get(key);
      currentConfig?.onError?.(new Error('WebSocket error'));
    };

    ws.onclose = (event) => {
      console.log(`SSH WebSocket closed for key ${key}, code: ${event.code}, reason: ${event.reason}`);
      this.connections.delete(key);
      const currentConfig = this.configs.get(key);
      this.configs.delete(key);
      currentConfig?.onClose?.();
    };

    // Add send method for input
    (ws as any).sendInput = (input: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        const message = JSON.stringify({ type: 'input', data: input });
        console.log('WebSocket sending:', message);
        ws.send(message);
      } else {
        console.warn('WebSocket not open, cannot send input. State:', ws.readyState);
      }
    };

    return ws;
  }

  closeConnection(key: string, code?: number, reason?: string) {
    const ws = this.connections.get(key);
    if (ws && ws.readyState !== WebSocket.CLOSED) {
      ws.close(code || 1000, reason || 'Normal closure');
      this.connections.delete(key);
      this.configs.delete(key);
    }
  }

  isConnected(key: string): boolean {
    const ws = this.connections.get(key);
    return ws ? ws.readyState === WebSocket.OPEN : false;
  }

  getConnectionState(key: string): number | null {
    const ws = this.connections.get(key);
    return ws ? ws.readyState : null;
  }
}
import { AlertCircle, Maximize2, Minimize2, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Terminal } from "xterm";
import { FitAddon } from "xterm-addon-fit";
import { WebLinksAddon } from "xterm-addon-web-links";
import type { SSHMessage } from "../api/infrastructure";
import { SSHWebSocketManager } from "../utils/sshWebSocketManager";
import { Button } from "./ui/button";
import "xterm/css/xterm.css";

interface SSHTerminalProps {
	env: string;
	serviceName: string;
	taskArn: string;
	containerName?: string;
	onClose: () => void;
}

export function SSHTerminal({
	env,
	serviceName,
	taskArn,
	containerName,
	onClose,
}: SSHTerminalProps) {
	const terminalRef = useRef<HTMLDivElement>(null);
	const terminalInstance = useRef<Terminal | null>(null);
	const fitAddon = useRef<FitAddon | null>(null);
	const wsManager = useRef(SSHWebSocketManager.getInstance());
	const connectionKey = useRef(`${env}-${serviceName}-${taskArn}`);
	const connectionStatusRef = useRef<
		"connecting" | "connected" | "disconnected" | "error"
	>("connecting");
	const [isFullScreen, setIsFullScreen] = useState(true);
	const [connectionStatus, setConnectionStatus] = useState<
		"connecting" | "connected" | "disconnected" | "error"
	>("connecting");
	const [error, setError] = useState<string | null>(null);
	const isInitialized = useRef(false);

	// Update connection status ref when state changes
	useEffect(() => {
		connectionStatusRef.current = connectionStatus;
	}, [connectionStatus]);

	const updateConnectionStatus = (status: typeof connectionStatus) => {
		connectionStatusRef.current = status;
		setConnectionStatus(status);
	};

	useEffect(() => {
		if (!terminalRef.current || isInitialized.current) return;

		// Ensure the container is visible and has dimensions
		const checkAndInitialize = () => {
			if (!terminalRef.current) return;

			const rect = terminalRef.current.getBoundingClientRect();
			if (rect.width === 0 || rect.height === 0) {
				// Container not ready yet, try again
				setTimeout(checkAndInitialize, 50);
				return;
			}

			// Mark as initialized to prevent multiple attempts
			isInitialized.current = true;

			// Initialize terminal with default dimensions
			const terminal = new Terminal({
				fontSize: 14,
				fontFamily: 'Menlo, Monaco, "Courier New", monospace',
				theme: {
					background: "#0c0c0c",
					foreground: "#d0d0d0",
					cursor: "#d0d0d0",
					black: "#000000",
					red: "#ff5555",
					green: "#50fa7b",
					yellow: "#f1fa8c",
					blue: "#bd93f9",
					magenta: "#ff79c6",
					cyan: "#8be9fd",
					white: "#bbbbbb",
					brightBlack: "#555555",
					brightRed: "#ff5555",
					brightGreen: "#50fa7b",
					brightYellow: "#f1fa8c",
					brightBlue: "#bd93f9",
					brightMagenta: "#ff79c6",
					brightCyan: "#8be9fd",
					brightWhite: "#ffffff",
				},
				cursorBlink: true,
				allowTransparency: true,
				rows: 24,
				cols: 80,
			});

			// Add addons
			const fit = new FitAddon();
			const webLinks = new WebLinksAddon();

			terminal.loadAddon(fit);
			terminal.loadAddon(webLinks);

			terminalInstance.current = terminal;
			fitAddon.current = fit;

			// Open terminal
			terminal.open(terminalRef.current);

			// Wait for DOM to be ready and terminal to be attached
			requestAnimationFrame(() => {
				// Ensure the terminal container has dimensions
				if (
					terminalRef.current &&
					terminalRef.current.offsetWidth > 0 &&
					terminalRef.current.offsetHeight > 0
				) {
					try {
						fit.fit();
						// Force a refresh after fitting
						terminal.refresh(0, terminal.rows - 1);
					} catch (err) {
						console.error("Error fitting terminal:", err);
						// Try again after a short delay
						setTimeout(() => {
							try {
								fit.fit();
								terminal.refresh(0, terminal.rows - 1);
							} catch (e) {
								console.error("Second attempt to fit terminal failed:", e);
							}
						}, 100);
					}
				} else {
					console.warn("Terminal container has no dimensions, retrying...");
					setTimeout(() => {
						if (
							terminalRef.current &&
							terminalRef.current.offsetWidth > 0 &&
							terminalRef.current.offsetHeight > 0
						) {
							try {
								fit.fit();
								terminal.refresh(0, terminal.rows - 1);
							} catch (err) {
								console.error("Delayed fit attempt failed:", err);
							}
						}
					}, 100);
				}
			});

			// Show connecting message
			terminal.writeln("\x1b[1;36mConnecting to SSH session...\x1b[0m\n");

			// Build WebSocket URL
			const params = new URLSearchParams({
				env,
				service: serviceName,
				taskArn,
			});
			if (containerName) {
				params.append("container", containerName);
			}

			// Determine WebSocket URL - use the API base URL if available
			let wsUrl: string;
			const apiBaseUrl = import.meta.env.VITE_API_URL || "";

			if (apiBaseUrl) {
				// Convert http/https to ws/wss
				wsUrl = `${apiBaseUrl.replace(/^http/, "ws")}/ws/ssh?${params}`;
			} else {
				// Default to backend on port 8080
				const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
				wsUrl = `${protocol}//localhost:8080/ws/ssh?${params}`;
			}

			console.log("SSHTerminal: Attempting to connect with URL:", wsUrl);
			console.log("SSHTerminal: Connection key:", connectionKey.current);

			// Connect using WebSocket manager
			const ws = wsManager.current.getConnection(connectionKey.current, {
				url: wsUrl,
				onOpen: () => {
					console.log("SSH WebSocket opened - sending connected status");
					// The backend should send a 'connected' message, but let's check if it's already connected
					if (wsManager.current.isConnected(connectionKey.current)) {
						console.log(
							"WebSocket is confirmed open, waiting for backend connected message",
						);
					}
				},
				onMessage: (message: SSHMessage) => {
					console.log("SSH message received:", message);
					// Use the ref to ensure we have the current terminal instance
					const currentTerminal = terminalInstance.current;
					if (!currentTerminal) {
						console.error("Terminal not initialized when message received");
						return;
					}

					switch (message.type) {
						case "connected":
							clearTimeout(connectionTimeout);
							updateConnectionStatus("connected");
							// Don't clear the terminal - preserve the connection message
							currentTerminal.writeln(
								"\n\x1b[1;32m✓ SSH session connected!\x1b[0m",
							);
							if (message.data) {
								currentTerminal.writeln(`\x1b[1;36m${message.data}\x1b[0m`);
							}
							break;
						case "output":
							currentTerminal.write(message.data);
							break;
						case "error":
							updateConnectionStatus("error");
							setError(message.data);
							currentTerminal.writeln(
								`\n\x1b[1;31mError: ${message.data}\x1b[0m`,
							);
							break;
						case "disconnected":
							updateConnectionStatus("disconnected");
							currentTerminal.writeln(
								"\n\x1b[1;33mSSH session disconnected.\x1b[0m",
							);
							break;
					}
				},
				onError: (error) => {
					updateConnectionStatus("error");
					const errorMsg =
						error.message || "Failed to establish WebSocket connection";
					setError(errorMsg);
					const currentTerminal = terminalInstance.current;
					if (currentTerminal) {
						currentTerminal.writeln(
							`\n\x1b[1;31mConnection error: ${errorMsg}\x1b[0m`,
						);
						currentTerminal.writeln(
							`\x1b[1;33mPlease ensure the backend server is running and accessible.\x1b[0m`,
						);
					}
				},
				onClose: () => {
					updateConnectionStatus("disconnected");
					const currentTerminal = terminalInstance.current;
					if (currentTerminal) {
						currentTerminal.writeln("\n\x1b[1;33mConnection closed.\x1b[0m");
					}
				},
			});

			// Add connection timeout
			const connectionTimeout = setTimeout(() => {
				if (!wsManager.current.isConnected(connectionKey.current)) {
					console.error("SSH connection timeout after 10 seconds");
					updateConnectionStatus("error");
					setError("Connection timeout - backend may not be responding");
					const currentTerminal = terminalInstance.current;
					if (currentTerminal) {
						currentTerminal.writeln(
							"\n\x1b[1;31mConnection timeout after 10 seconds\x1b[0m",
						);
						currentTerminal.writeln(
							"\x1b[1;33mThe backend server may not have the SSH endpoint implemented.\x1b[0m",
						);
					}
				}
			}, 10000);

			// Handle terminal input
			terminal.onData((data) => {
				console.log(
					"Terminal input:",
					JSON.stringify(data),
					"Connection status:",
					connectionStatusRef.current,
				);
				// Use the manager to check connection and send data
				if (
					wsManager.current.isConnected(connectionKey.current) &&
					connectionStatusRef.current === "connected"
				) {
					console.log("Sending input to WebSocket:", JSON.stringify(data));
					(ws as any).sendInput(data);
				}
			});

			// Handle window resize
			const handleResize = () => {
				if (fitAddon.current && terminalInstance.current) {
					try {
						fitAddon.current.fit();
					} catch (err) {
						console.error("Error resizing terminal:", err);
					}
				}
			};
			window.addEventListener("resize", handleResize);

			// Cleanup
			return () => {
				clearTimeout(connectionTimeout);
				window.removeEventListener("resize", handleResize);
				terminal.dispose();
				// Only close connection in production or when explicitly closing the terminal
				if (process.env.NODE_ENV === "production") {
					wsManager.current.closeConnection(connectionKey.current);
				}
			};
		}; // End of checkAndInitialize function

		// Start the initialization process
		checkAndInitialize();

		// Don't re-run effect on prop changes since we're managing initialization
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, []);

	// Handle fullscreen toggle
	const toggleFullScreen = () => {
		setIsFullScreen(!isFullScreen);
		setTimeout(() => {
			if (fitAddon.current && terminalInstance.current) {
				try {
					fitAddon.current.fit();
				} catch (err) {
					console.error("Error fitting terminal on fullscreen toggle:", err);
				}
			}
		}, 100);
	};

	// Handle component close
	const handleClose = () => {
		// Always close the connection when user explicitly closes the terminal
		wsManager.current.closeConnection(connectionKey.current);
		onClose();
	};

	// Get task ID from ARN for display
	const taskId = taskArn.split("/").pop() || taskArn;

	return (
		<div
			className={`fixed ${isFullScreen ? "inset-0" : "inset-4"} z-50 bg-gray-950 flex flex-col transition-all duration-200`}
		>
			{/* Header */}
			<div className="flex items-center justify-between p-4 border-b border-gray-700 bg-gray-900">
				<div className="flex items-center gap-4">
					<h2 className="text-lg font-semibold text-white">
						SSH Terminal - {serviceName}
					</h2>
					<div className="flex items-center gap-2">
						<span className="text-xs text-gray-400">
							Task: {taskId.substring(0, 12)}...
						</span>
						{connectionStatus === "connecting" && (
							<span className="text-xs px-2 py-1 bg-yellow-800 text-yellow-300 rounded animate-pulse">
								Connecting...
							</span>
						)}
						{connectionStatus === "connected" && (
							<span className="text-xs px-2 py-1 bg-green-800 text-green-300 rounded">
								Connected
							</span>
						)}
						{connectionStatus === "disconnected" && (
							<span className="text-xs px-2 py-1 bg-gray-700 text-gray-300 rounded">
								Disconnected
							</span>
						)}
						{connectionStatus === "error" && (
							<span className="text-xs px-2 py-1 bg-red-800 text-red-300 rounded">
								Error
							</span>
						)}
					</div>
				</div>

				<div className="flex items-center gap-2">
					<Button size="sm" variant="ghost" onClick={toggleFullScreen}>
						{isFullScreen ? (
							<Minimize2 className="w-4 h-4" />
						) : (
							<Maximize2 className="w-4 h-4" />
						)}
					</Button>
					<Button size="sm" variant="ghost" onClick={handleClose}>
						<X className="w-4 h-4" />
					</Button>
				</div>
			</div>

			{/* Error Message */}
			{error && (
				<div className="p-4 bg-red-900/20 border-b border-red-700">
					<div className="flex items-center gap-2">
						<AlertCircle className="w-4 h-4 text-red-400" />
						<span className="text-sm text-red-300">{error}</span>
					</div>
				</div>
			)}

			{/* Terminal */}
			<div
				className="flex-1 overflow-hidden bg-black p-4"
				style={{ minHeight: "300px" }}
			>
				<div
					ref={terminalRef}
					className="h-full w-full"
					style={{
						backgroundColor: "#0c0c0c",
						minHeight: "200px",
						minWidth: "400px",
						height: "100%",
						width: "100%",
						position: "relative",
						display: "block",
					}}
				>
					{/* Show loading indicator if terminal not initialized */}
					{!terminalInstance.current && (
						<div className="flex items-center justify-center h-full text-gray-500">
							<div className="text-center">
								<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-400 mx-auto mb-2"></div>
								<p className="text-sm">Initializing terminal...</p>
							</div>
						</div>
					)}
				</div>
			</div>

			{/* Status Bar */}
			<div className="flex items-center justify-between px-4 py-2 bg-gray-900 border-t border-gray-700">
				<div className="flex items-center gap-4 text-xs text-gray-400">
					<span>Environment: {env}</span>
					<span>Service: {serviceName}</span>
					{containerName && <span>Container: {containerName}</span>}
				</div>
				<div className="text-xs text-gray-500">
					Press Ctrl+C to interrupt • Ctrl+D to exit
				</div>
			</div>
		</div>
	);
}

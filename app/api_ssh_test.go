package main

import (
	"fmt"
	"net/http"
	"time"
)

// testWebSocket is a simple echo WebSocket for testing
func testWebSocket(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Test WebSocket connection request\n")

	// Upgrade to WebSocket
	conn, err := sshUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("Test WebSocket upgrade failed: %v\n", err)
		return
	}
	fmt.Printf("Test WebSocket upgrade successful\n")
	defer conn.Close()

	// Send initial message
	if err := conn.WriteJSON(map[string]string{
		"type": "connected",
		"data": "Test WebSocket connected successfully",
	}); err != nil {
		fmt.Printf("Failed to send test connected message: %v\n", err)
		return
	}

	// Echo loop
	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			fmt.Printf("Test WebSocket read error: %v\n", err)
			breakwhen
		}

		fmt.Printf("Test WebSocket received: %v\n", msg)

		// Echo back with timestamp
		response := map[string]interface{}{
			"type":      "echo",
			"data":      msg,
			"timestamp": time.Now().Format(time.RFC3339),
		}

		if err := conn.WriteJSON(response); err != nil {
			fmt.Printf("Test WebSocket write error: %v\n", err)
			break
		}
	}

	fmt.Printf("Test WebSocket connection closed\n")
}

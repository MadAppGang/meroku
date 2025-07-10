# Debug Position Tool

## Overview
A debug button has been added to the canvas controls to help troubleshoot node positioning and edge connection issues in the infrastructure visualization.

## Location
The debug button (bug icon) appears at the bottom of the left toolbar in the canvas view.

## Features

### 1. Toggle Debug Mode
- Click the bug icon to toggle debug mode on/off
- When ON: The button appears red/highlighted
- When OFF: The button appears gray

### 2. Position Logging
Every time you click the debug button, it logs to the browser console:
- All node positions (x, y coordinates)
- All edge connections (source, target, handles)
- A JSON summary for easy copying
- Current debug mode state

### 3. Verbose Mode
When debug mode is ON:
- All position save operations are logged in detail
- All position load operations are logged in detail
- Helps track when positions are being saved/loaded from the backend

## Usage

1. **To Debug Missing Nodes:**
   - Click the debug button
   - Check the console for the list of nodes
   - Compare with expected nodes from your configuration

2. **To Debug Position Saving:**
   - Enable debug mode (button turns red)
   - Move a node
   - Check console for "DEBUG: All positions being saved" messages

3. **To Debug Position Loading:**
   - Enable debug mode
   - Refresh the page or switch environments
   - Check console for "DEBUG: All loaded positions" messages

4. **To Copy Node Positions:**
   - Click the debug button
   - In the console, find the JSON summary
   - Right-click and copy the position data you need

## Console Output Example
```
=== DEBUG MODE: ENABLED ===
Verbose position logging is now enabled

=== DEBUG: Node Positions ===
Total nodes: 25
Node: github
  Position: x=230, y=-80
  Type: service
  Data: {id: "github", type: "github", ...}
...

=== DEBUG: Edge Connections ===
Total edges: 15
Edge: github-ecr
  Source: github (handle: source-bottom)
  Target: ecr (handle: target-top)
  Type: smoothstep
  Label: push
...

=== DEBUG: Summary (JSON) ===
{
  "nodes": [...],
  "edges": [...]
}
```

## Troubleshooting Tips

1. **Nodes not appearing where expected:**
   - Enable debug mode and refresh
   - Check if positions are being loaded from backend
   - Verify node IDs match between saves and loads

2. **Positions not saving:**
   - Enable debug mode
   - Move a node and wait 0.5 seconds
   - Check console for save operations
   - Look for any error messages

3. **Edge connections incorrect:**
   - Click debug button to see all edge handles
   - Verify sourceHandle and targetHandle values
   - Check if custom handles are being saved/loaded
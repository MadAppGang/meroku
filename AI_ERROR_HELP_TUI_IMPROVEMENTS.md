# AI Error Help TUI Improvements

## Summary

Enhanced the Modern Plan TUI to show AI error help **within the TUI** instead of exiting to terminal. Users can now view error analysis, suggested fixes, and scroll through the content without losing their visual context.

## Problem Solved

**Before**: When deployment failed and user pressed 'a' for AI help:
1. TUI would quit
2. AI suggestions shown in terminal
3. User loses visual error context
4. Can't easily reference both error and suggestions

**After**: When deployment failed and user presses 'a' for AI help:
1. TUI stays open
2. Switches to AI Help view showing:
   - ❌ Original Error (in red)
   - 💡 AI Analysis (AI's explanation)
   - 📋 Suggested Fix (commands to run)
3. Scrollable content
4. Press ESC to go back to Apply view
5. Keep all context visible

## Implementation

### New View Mode
Added `aiHelpView` to the view modes:
```go
const (
    dashboardView viewMode = iota
    applyView
    fullScreenDiffView
    aiHelpView  // NEW!
)
```

### New Model Fields
```go
// AI help view
aiHelpProblem   string          // AI's problem analysis
aiHelpCommands  []string        // Suggested fix commands
aiHelpErrors    []string        // Original error messages
aiHelpViewport  viewport.Model  // Scrollable content
```

### Key Handler Changes
**Apply Key ('a') in error state**:
- **Before**: Set flag, quit TUI, show in terminal
- **After**: Fetch AI suggestions, switch to AI help view (stay in TUI)

**ESC Key**:
- Added handling to go back from AI help view to apply view

**Up/Down Keys**:
- Added scrolling support for AI help view content

### New Functions

#### `fetchAIHelpAndShowView()`
- Collects error messages from failed resources
- Calls `getAIErrorSuggestions()` to get AI analysis
- Stores results in model fields
- Switches to `aiHelpView`
- Initializes scrollable viewport with content

#### `buildAIHelpContent()`
- Formats the AI help content with sections:
  - Original Error (red, from apply logs)
  - AI Analysis (yellow, AI's explanation)
  - Suggested Fix (green, commands to run)
  - Disclaimer (gray, warning about AI-generated content)

#### `renderAIHelpView()`
- Renders the AI help view with:
  - Header: "🤖 AI Error Help"
  - Scrollable viewport with formatted content
  - Footer: "[↑↓] Scroll • [ESC] Back to Apply View"

## User Experience

### Apply View with Errors
Footer shows: `[a] AI Help • [Enter] Continue  [Ctrl+C] Force Stop`

### When User Presses 'a'
1. AI analysis runs (brief moment)
2. TUI switches to AI Help view
3. Shows three sections:
   ```
   ❌ Original Error
   ─────────────────
   Resource: module.workloads.aws_service_discovery_service.backend[0]
   Action: create
   Error: creating Service Discovery Service (sava_service_prod): operation error
   ServiceDiscovery: CreateService, https response error StatusCode: 400...

   💡 AI Analysis
   ─────────────────
   The error indicates that a Service Discovery service already exists with
   the same name. This typically happens when...

   📋 Suggested Fix
   ─────────────────
   Run these commands:
     cd env/dev
     export AWS_PROFILE=your-profile
     aws servicediscovery delete-service --id srv-xyz123
     terraform apply

   ⚠️  AI-generated suggestions - please review before running
   ```

### Navigation
- **↑↓ Arrow Keys**: Scroll content
- **ESC**: Go back to Apply view
- **Ctrl+C**: Quit entirely

## Benefits

1. **Context Preservation**: Error details remain visible in TUI history
2. **Better UX**: No jarring context switch from TUI to terminal
3. **Scrollable**: Can review long error messages and commands
4. **Consistent Interface**: Matches existing full-screen diff view pattern
5. **Backward Compatible**: Users who press Enter still get prompted for AI help after exit

## Files Modified

- `app/terraform_plan_modern_tui.go`
  - Added `aiHelpView` mode
  - Added AI help fields to model
  - Modified Apply key handler
  - Added ESC/scroll handlers for AI help view
  - Added 3 new functions for AI help view
- `app/ai_helper.go`
  - Added `displayAISuggestionsWithContext()` wrapper function

## Future Enhancements

Possible improvements:
1. Add [C] Copy Commands to clipboard
2. Add syntax highlighting for commands
3. Show "Analyzing..." spinner while AI processes
4. Extend to Plan Progress and Destroy Progress TUIs
5. Add [R] Run command directly from TUI (with confirmation)

## Testing

To test:
1. Set `ANTHROPIC_API_KEY` environment variable
2. Deploy something that will fail (e.g., duplicate resource)
3. When apply completes with errors, press 'a'
4. Verify AI help view displays
5. Test scrolling with arrow keys
6. Test ESC to go back
7. Test Enter to exit and get prompted help (backward compat)

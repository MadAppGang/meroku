# Nuke TUI Refactoring - Complete Architectural Redesign

## Problem
The nuke TUI was trying to be both a multi-stage confirmation wizard AND a live Terraform output viewer in a single TUI model. This created race conditions and synchronization issues that prevented Terraform output from displaying properly.

## Solution: Separation of Concerns

### Before (Monolithic Approach)
```
nukeMenu()
  └─> Single nukeModel TUI
      ├─> Stage 1-4: Confirmation wizard
      ├─> Stage 5: Run terraform destroy inline
      │   └─> Try to show output in same TUI
      └─> Stage 6-7: Complete/Cancelled
```

**Issues:**
- Mixed concerns: wizard logic + terraform execution
- Race conditions with goroutines writing to arrays
- Message passing delays causing empty output
- Complex state management

### After (Clean Separation)
```
nukeMenu()
  ├─> nukeModel TUI (Confirmation Wizard Only)
  │   ├─> Stage 1: Select Environment
  │   ├─> Stage 2: Show Details
  │   ├─> Stage 3: First Confirmation (Yes/No)
  │   ├─> Stage 4: Project Name Verification
  │   └─> Stage 5: Confirmed → QUIT
  │
  └─> If confirmed:
      └─> runTerraformDestroyWithProgress(env)
          └─> Dedicated destroyProgressModel TUI
              └─> Live Terraform output with bordered box
```

## Key Changes

### 1. Simplified nukeModel (nuke_tui.go)
**Removed:**
- `destroyingStage`, `completeStage` (only need `confirmedStage`)
- All destroy execution logic (`runDestroy`, `startTerraformDestroy`)
- Output tracking fields (`destroyOutput`, `outputMutex`, `destroyedCount`, etc.)
- Progress message types (`nukeProgressMsg`, `nukeTickMsg`, etc.)
- View functions for destroying and complete stages

**Kept:**
- Clean wizard stages (select, details, confirm, verify)
- User input handling
- Environment selection logic

### 2. Updated nukeMenu() (nuke.go)
**Now:**
1. Runs the confirmation wizard (nukeModel)
2. Checks the final stage
3. If `confirmedStage`: launches `runTerraformDestroyWithProgress(env)`
4. Otherwise: returns to main menu

### 3. Reuses Existing Working Code
The destroy operation now uses the **exact same TUI** as the regular destroy command:
- `runTerraformDestroyWithProgress()` from terraform_destroy_progress_tui.go
- Already has all the features: bordered output box, dynamic sizing, live streaming
- Proven to work reliably

## Benefits

### ✅ Reliability
- No more race conditions
- Uses proven, working code
- Direct mutex-protected writes
- No message passing delays

### ✅ Consistency
- Nuke destroy looks identical to regular destroy
- Same bordered "Live Output:" box
- Same color coding and formatting
- Same user experience across all destroy operations

### ✅ Maintainability
- Single responsibility per TUI
- Less code duplication
- Easier to debug and test
- Clear separation of concerns

### ✅ User Experience
- Full Terraform output visibility
- Dynamic terminal sizing
- Professional bordered output box
- Real-time streaming

## Code Flow

### User Journey
```
1. User selects "Nuke Environment"
   ↓
2. Nuke Wizard TUI starts
   - Select environment from list
   - View environment details
   - Confirm with Yes/No
   - Type project name to verify
   ↓
3. Wizard quits with confirmedStage
   ↓
4. nukeMenu() detects confirmation
   ↓
5. Launches Destroy Progress TUI
   - Shows "🔥 Proceeding with destruction..."
   - Displays bordered output box
   - Streams all Terraform output live
   - Shows spinner and progress
   ↓
6. Returns to main menu
   - Success message
   - Preserves config files
```

### Technical Flow
```go
// nuke.go
func nukeMenu() {
    model := initNukeTUI(env)
    p := tea.NewProgram(model, tea.WithAltScreen())
    finalModel, _ := p.Run()

    if finalModel.stage == confirmedStage {
        // Launch dedicated destroy TUI
        runTerraformDestroyWithProgress(env)
    }
}
```

## Files Changed

1. **app/nuke_tui.go** - Simplified to wizard-only
   - Removed: ~250 lines of destroy execution code
   - Cleaned: imports, message types, stages, view functions

2. **app/nuke.go** - Added destroy TUI launcher
   - Added: stage check and progress TUI call
   - Added: success/failure messages

3. **app/terraform_destroy_progress_tui.go** - No changes needed!
   - Already working perfectly
   - Reused as-is

## Testing
```bash
# Build
cd app && go build -o ../meroku

# Run
./meroku
# Select "Nuke Environment"
# Follow the wizard
# Observe full Terraform output in bordered box
```

## Comparison with Deploy Flow

This now matches the deploy operation's architecture:

### Deploy (Already working)
```
Deploy Menu
  └─> runTerraformApply()
      ├─> runTerraformPlanWithProgress() [Dedicated TUI]
      ├─> showModernTerraformPlanTUI()    [Dedicated TUI]
```

### Destroy (Now fixed)
```
Nuke Menu
  └─> Confirmation Wizard [Dedicated TUI]
      └─> runTerraformDestroyWithProgress() [Dedicated TUI]
```

**Pattern:** Multi-step operations use multiple focused TUIs, each with a single responsibility.

---

*This refactoring follows the Unix philosophy: Do one thing and do it well. Each TUI now has a single, clear purpose.*

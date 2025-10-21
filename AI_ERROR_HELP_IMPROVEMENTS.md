# AI Error Help UX Improvements

## Summary

Enhanced the error handling UI across all TUI components to provide inline AI help options, eliminating the need for users to exit error screens before accessing AI assistance.

## Changes Made

### 1. Plan Progress TUI (`app/terraform_plan_progress_tui.go`)

**Import Added:**
- Added `"os"` import for environment variable access

**Model Changes:**
- Added `requestAIHelp bool` field to track AI help requests

**Footer Changes:**
```
Before: "Press Enter to return to menu or q to quit"
After:  "q - exit • a - try to fix with AI • Enter - return to menu"
```
(AI option only visible when `ANTHROPIC_API_KEY` is set)

**Key Handler:**
- Added 'a' key detection during error phase
- Sets `requestAIHelp = true` and quits TUI

**AI Integration:**
- Modified `runTerraformPlanWithProgress()` to check `requestAIHelp` flag
- Directly calls `getAIErrorSuggestions()` without prompting
- Displays suggestions immediately using `displayAISuggestions()`

### 2. Destroy Progress TUI (`app/terraform_destroy_progress_tui.go`)

**Model Changes:**
- Added `requestAIHelp bool` field

**Footer Changes:**
```
Before: "[E] Full Error • [C] Copy • Press any key to exit"
After:  "[E] Full Error • [C] Copy • [A] AI Help • Q - exit"
```
(AI option only visible when `ANTHROPIC_API_KEY` is set)

**Key Handler:**
- Added 'a' key handler in error state
- Sets `requestAIHelp = true` and quits TUI

**AI Integration:**
- Modified `runTerraformDestroyWithProgress()` to check `requestAIHelp`
- Calls AI suggestions directly when flag is set
- Maintains backward compatibility with prompted flow

### 3. Modern Plan TUI (`app/terraform_plan_modern_tui.go`)

**Model Changes:**
- Added `requestAIHelp bool` field to `modernPlanModel`

**Footer Changes:**
- Modified `renderApplyFooter()` to include "[a] AI Help •" option
- Only shown when apply is complete with errors and AI is available

**Key Handler:**
- Added `case msg.String() == "a"` handler
- Only active when in apply view with completed errors
- Sets `requestAIHelp = true` and quits

**AI Integration:**
- Updated `showModernTerraformPlanTUI()` to check `requestAIHelp`
- Collects error messages from completed resources
- Calls AI directly without prompting

## User Experience Flow

### Before
1. Error occurs
2. Error screen displays
3. User presses Enter
4. Returns to menu
5. Prompted: "Would you like AI help? (y/n)"
6. User types 'y' or 'n'
7. AI suggestions displayed (if yes)

### After
1. Error occurs
2. Error screen displays with options: "q - exit • a - try to fix with AI • Enter - return to menu"
3. User presses 'a'
4. AI immediately analyzes and displays suggestions
5. User presses Enter to continue

## Benefits

1. **Faster Access**: Reduced from 3+ interactions to 1 key press
2. **Clear Instructions**: Users see available options without guessing
3. **Consistent UX**: All three TUI types have the same error handling pattern
4. **Backward Compatible**: Exiting without 'a' still offers prompted AI help
5. **Smart Display**: AI option only appears when API key is configured
6. **Non-Intrusive**: Users who don't want AI help can easily exit with 'q' or Enter

## Testing

To test the changes:

1. Set `ANTHROPIC_API_KEY` environment variable
2. Trigger a terraform error (plan or destroy)
3. Verify AI help option appears in footer
4. Press 'a' to get immediate AI suggestions
5. Verify suggestions are displayed without additional prompts

## Files Modified

- `app/terraform_plan_progress_tui.go` (plan errors)
- `app/terraform_destroy_progress_tui.go` (destroy errors)
- `app/terraform_plan_modern_tui.go` (apply errors)

All changes maintain backward compatibility and do not affect users without the AI API key configured.

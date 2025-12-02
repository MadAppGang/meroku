# Implementation Plan: Custom Terraform Editor with Monaco

## Overview
This plan outlines the implementation of a Custom Terraform Editor within the Meroku web interface. The feature allows users to create, edit, and manage custom HCL files alongside the visual infrastructure builder. It leverages the Monaco Editor for a rich coding experience with Terraform syntax highlighting and project-specific autocompletion.

## Architecture & Design

### Component Hierarchy
```
App (src/App.tsx)
├── TopPanel (Navigation Control added here)
├── ViewWrapper (Visual Builder) - Always mounted, toggled via CSS
│   ├── DeploymentCanvas
│   └── Sidebar (Properties)
└── ViewWrapper (Custom Terraform Editor) - Always mounted, toggled via CSS
    └── CustomTerraformManager (src/components/CustomTerraformManager.tsx)
        ├── Sidebar (File List)
        │   └── FileList / NewFileDialog
        ├── TerraformEditor (src/components/TerraformEditor.tsx)
        │   └── MonacoEditor (@monaco-editor/react)
        └── BridgeVariablesPanel (Right Sidebar)
```

### State Management
- **App.tsx**: Will manage a new `viewMode` state (`"visual" | "code"`) to toggle visibility of the two main views.
- **CustomTerraformManager**: Will manage its own local state for file lists, selected file content, and dirty state (unsaved changes).

### Data Flow
1. **API Layer**: `src/api/customTerraform.ts` handles communication with backend endpoints.
2. **Bridge Variables**: Fetched on mount by `CustomTerraformManager` to populate the autocomplete engine in `TerraformEditor`.
3. **File Operations**: Save/Delete operations trigger API calls and local state updates.

---

## Implementation Phases

### Phase 1: Foundation & Assets (Frontend)
*Goal: Establish the core components and API client.*

1. **API Client & Types**:
   - Create `src/types/customTerraform.ts` with interfaces for Files, BridgeVariables, and API responses.
   - Create `src/api/customTerraform.ts` implementing `listFiles`, `getFile`, `saveFile`, `deleteFile`, `getBridgeVariables`.
   - **Validation**: Ensure API client handles 400/500 errors gracefully with user-friendly toast messages.

2. **Editor Component**:
   - Create `src/components/TerraformEditor.tsx`.
   - **Performance**: Use `@monaco-editor/react` with lazy loading to avoid bloating initial bundle.
   - Configure Monaco Editor interaction.
   - Implement `handleEditorDidMount` to register `hcl` language support.
   - Implement custom completion provider for `local.bridge.*` variables.
   - **Performance**: Ensure the completion provider is debounced or optimized if variable list is large.

3. **Manager Component**:
   - Create `src/components/CustomTerraformManager.tsx`.
   - Implement File Tree sidebar (Shared vs Environment scopes).
   - Implement Main Editor area.
   - Implement "Bridge Variables" reference sidebar.
   - Handle "New File", "Save", "Delete" flows with Dialogs.
   - **Validation**: Validate filenames (regex `^[a-zA-Z0-9_-]+$`) before sending to API.

### Phase 2: Integration
*Goal: Connect the new feature to the main application while preserving state.*

1. **App.tsx State Updates**:
   - Add `const [viewMode, setViewMode] = useState<"visual" | "code">("visual");`.
   - **State Preservation**: Instead of conditional rendering (`{viewMode === 'visual' ? ... : ...}`), use **CSS toggling** (`className={viewMode === 'visual' ? 'block' : 'hidden'}`) to keep the `DeploymentCanvas` mounted. This preserves zoom/pan/selection state when user switches to Code view.

2. **Navigation UI**:
   - Modify `src/components/TopPanel.tsx`.
   - Add a toggle/tab mechanism to switch between "Visual Builder" and "Custom Terraform".
   - **UX**: Before switching views, check for unsaved changes in `CustomTerraformManager` (if possible, or via a dirty bit lifted to App state) and warn user.

3. **Styling & Refinement**:
   - Ensure dark mode consistency (Tailwind classes).
   - Verify Monaco Editor theme matches application theme.

---

## Detailed File Changes

### 1. `src/types/customTerraform.ts` (New)
Define the shape of data structures.
```typescript
export interface CustomTerraformFile {
  path: string;
  name: string;
  content: string;
  scope: "shared" | "environment";
  // ...
}
// ... BridgeVariable, API Response interfaces
```

### 2. `src/api/customTerraform.ts` (New)
Implement the fetch wrapper for the new endpoints. Use existing `fetchWithTokenRetry` utility.

### 3. `src/components/TerraformEditor.tsx` (New)
Wraps `@monaco-editor/react`.
- **Critical**: Register `monaco.languages.registerCompletionItemProvider` for `hcl` language.
- **Logic**:
  - Trigger on `.` character.
  - If current line contains `local.bridge`, filter `bridgeVariables` prop.
  - Return suggestions with `insertText` and documentation.

### 4. `src/components/CustomTerraformManager.tsx` (New)
The container view.
- **Props**: `environment: string`.
- **Effects**: Fetch files and bridge vars when `environment` changes.
- **UI Layout**: Flexbox layout with 3 columns: `FileTree (250px) | Editor (Flex) | BridgeVars (300px)`.

### 5. `src/App.tsx` (Modification)
Logic to switch views using CSS visibility to preserve state.

```tsx
// Add state
const [viewMode, setViewMode] = useState<"visual" | "code">("visual");

// Render logic modification (CSS Toggle)
<div className="flex-1 flex flex-col overflow-hidden relative">
  {/* Visual Builder - Always mounted, hidden if not active */}
  <div className={viewMode === "visual" ? "flex-1 flex flex-col h-full" : "hidden"}>
    <ReactFlowProvider>
      <DeploymentCanvas ... />
      <Sidebar ... />
    </ReactFlowProvider>
  </div>

  {/* Code Editor - Always mounted (lazy load content), hidden if not active */}
  <div className={viewMode === "code" ? "flex-1 flex flex-col h-full bg-gray-900" : "hidden"}>
    <CustomTerraformManager environment={selectedEnvironment} />
  </div>
</div>
```

### 6. `src/components/TopPanel.tsx` (Modification)
Add the Switcher UI.

```tsx
// Add prop
viewMode: "visual" | "code";
onViewModeChange: (mode: "visual" | "code") => void;

// Add UI Element (e.g., next to Project/Region info)
<div className="flex bg-gray-900 rounded-md p-1 border border-gray-700">
  <button
    onClick={() => onViewModeChange("visual")}
    className={cn("px-3 py-1 text-xs rounded", viewMode === "visual" ? "bg-blue-600 text-white" : "text-gray-400 hover:text-white")}
  >
    Visual
  </button>
  <button
    onClick={() => onViewModeChange("code")}
    className={cn("px-3 py-1 text-xs rounded", viewMode === "code" ? "bg-blue-600 text-white" : "text-gray-400 hover:text-white")}
  >
    Code
  </button>
</div>
```

---

## Testing Strategy

### Manual Testing
1. **Navigation**: Verify switching between Visual and Code views preserves state (canvas zoom/pan position should NOT reset).
2. **File Operations**:
   - Create a new file (Environment scope). Verify it appears in list.
   - Create a new file (Shared scope). Verify it appears.
   - Edit and Save. Reload page to verify persistence.
   - Delete file. Verify removal.
3. **Editor Experience**:
   - Type `local.bridge.` and verify autocomplete dropdown appears.
   - Check syntax highlighting for standard Terraform keywords (`resource`, `variable`).
   - Verify layout on different screen sizes.
4. **Validation**:
   - Try creating file with invalid characters. Should see error.
   - Try navigating away with unsaved changes. Should see warning.

### Integration Tests
- Ensure `saveConfigToBackend` in `App.tsx` doesn't conflict with `saveFile` in `CustomTerraformManager`.

## Deployment & Rollback
- **Deployment**: Standard frontend build (`npm run build`).
- **Rollback**: Revert to previous commit. Feature is isolated in new components, so risk is low. `App.tsx` changes are minimal toggles.

## Risk Assessment
- **Low Risk**: The feature adds new capabilities without modifying the core `DeploymentCanvas` logic.
- **Dependency**: Relies on `monaco-editor`. Use lazy loader.
- **State Loss**: Addressed by CSS toggling strategy.

## Time Estimate
- Component Implementation: 4 hours
- Integration & Styling: 2 hours
- Testing & Polish: 2 hours
- **Total: ~1 Day**

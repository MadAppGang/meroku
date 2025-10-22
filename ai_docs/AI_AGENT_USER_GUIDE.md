# AI Agent User Guide

## What is the AI Agent?

The AI Agent is an autonomous troubleshooting system that can investigate and fix infrastructure deployment errors automatically. Unlike traditional error helpers that just suggest commands, the AI Agent:

- **Thinks** about what to investigate next
- **Acts** by running commands autonomously
- **Learns** from the results
- **Adapts** its strategy in real-time
- **Verifies** that fixes actually work

Think of it as having an expert DevOps engineer who can debug and fix issues while you watch.

## When to Use the AI Agent

### 1. After Terraform Errors (Automatic)

When `terraform apply` or `terraform destroy` fails, you'll be prompted:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🤖 Autonomous AI Agent Available!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

The AI agent will:
  • Analyze the errors autonomously
  • Investigate AWS resources and configuration
  • Identify root causes
  • Apply fixes automatically
  • Verify the solution works

You'll see each step as it happens and can stop at any time.

   Start the AI agent? (y/n):
```

Press `y` to let the agent investigate.

### 2. From the Main Menu (Proactive)

If you're experiencing issues but haven't tried deploying yet:

1. Run `./meroku` to open the main menu
2. Select **"🤖 AI Agent - Troubleshoot Issues"**
3. Describe the problem you're experiencing
4. The agent will investigate and attempt to fix it

Example problems you can describe:
- "ECS service not starting"
- "DNS not resolving"
- "Database connection issues"
- "Lambda function timing out"

## Prerequisites

### 1. Anthropic API Key

The agent requires an Anthropic API key to function:

```bash
export ANTHROPIC_API_KEY=your_key_here
```

Get your API key from: https://console.anthropic.com/settings/keys

### 2. AWS Credentials

Ensure you have valid AWS credentials configured:
- AWS profile selected in meroku
- Valid credentials (SSO or IAM user)
- Appropriate permissions for the resources being investigated

### 3. Environment Selected

Make sure you've selected an environment in meroku:
- Use **"🔄 Change Environment"** from the main menu
- Or let the deployment workflow handle environment selection

## How the Agent Works

### The Agent's Process

The agent uses an iterative debugging approach:

```
1. ANALYZE → Read error messages and context
2. INVESTIGATE → Check AWS resources, logs, configurations
3. DIAGNOSE → Identify the root cause
4. FIX → Apply necessary changes
5. VERIFY → Confirm the problem is solved
```

### What You'll See

When the agent runs, you'll see a real-time view of its thought process:

```
┌─────────────────────────────────────────────────────────────┐
│ 🤖 AI Agent - Autonomous Infrastructure Troubleshooter     │
│                                                             │
│ 🔧 Executing: aws ecs describe-services...                 │
│                                                             │
│ Iterations: 3 | Success: 2 | Failed: 0                     │
│                                                             │
│ ✓ 1. Checked ECS service status                            │
│    Action: aws_cli | Command: aws ecs describe-services... │
│    Output: Service has 0 running tasks                     │
│                                                             │
│ ✓ 2. Examined recent task failures                         │
│    Action: aws_cli | Command: aws ecs describe-tasks...    │
│    Output: Tasks failing with execution role error         │
│                                                             │
│ → 3. Updating terraform configuration... (running)          │
│    Action: file_edit                                        │
│                                                             │
│ [↑/↓] Navigate | [Enter] Details | [q] Stop agent          │
└─────────────────────────────────────────────────────────────┘
```

Each step shows:
- **Thought**: What the agent is thinking
- **Action**: What tool it's using (aws_cli, shell, file_edit, terraform_apply)
- **Command**: The exact command being run
- **Output**: What the command returned
- **Duration**: How long it took

### Agent Capabilities

The agent can:

**Investigate:**
- AWS resource status (ECS, RDS, Lambda, etc.)
- Configuration files (terraform, YAML)
- Logs and error messages
- IAM permissions
- Network connectivity

**Fix:**
- Edit terraform configurations
- Update YAML environment files
- Run terraform plan/apply
- Fix IAM roles and policies
- Adjust resource configurations

**Verify:**
- Check service health after changes
- Verify infrastructure state
- Confirm error resolution

## Using the Agent

### Example 1: ECS Service Not Starting

**Problem:** After terraform apply, ECS service shows 0 running tasks.

**Agent's Actions:**

```
Iteration 1: Check ECS service
→ Discovers service is active but has no running tasks

Iteration 2: Examine task definition
→ Finds task execution role ARN is empty

Iteration 3: Search terraform for IAM role
→ Confirms no ECS task execution role is defined

Iteration 4: Add IAM role to terraform
→ Updates env/dev/main.tf with proper IAM role

Iteration 5: Apply terraform changes
→ Runs terraform apply to create the role

Iteration 6: Force new deployment
→ Triggers ECS service to deploy with new role

Iteration 7: Verify service health
→ Confirms 1 task is running successfully

Result: Problem solved!
```

### Example 2: DNS Resolution Issues

**Problem:** Application can't connect to database.

**Agent's Actions:**

```
Iteration 1: Check RDS instance status
→ Instance is available and healthy

Iteration 2: Check security groups
→ Security group allows traffic from ECS

Iteration 3: Check Route53 records
→ Finds CNAME record is missing

Iteration 4: Search terraform configuration
→ Discovers Route53 zone is created but record is not

Iteration 5: Add DNS record to terraform
→ Updates configuration with CNAME record

Iteration 6: Apply terraform
→ Creates the missing DNS record

Iteration 7: Test DNS resolution
→ Confirms hostname resolves correctly

Result: Problem solved!
```

## Interacting with the Agent

### Navigation

- **↑/↓ or j/k**: Scroll through iterations
- **Enter**: Expand selected iteration (future feature)
- **q**: Stop the agent and return to menu

### Stopping the Agent

Press `q` at any time to stop the agent. Current iteration will complete, then the agent will exit gracefully.

**Note:** If the agent has made changes (e.g., edited files, applied terraform), those changes persist even if you stop the agent.

### Understanding Agent Status

**Status Icons:**
- `→` - Action is currently running
- `✓` - Action completed successfully
- `✗` - Action failed (agent will adapt)
- `💭` - Agent is thinking about the next step

**Progress Summary:**
```
Iterations: 5 | Success: 4 | Failed: 1
```
Shows total iterations, successful actions, and failed actions.

## Best Practices

### 1. Let the Agent Complete

While you can stop the agent at any time, it's best to let it complete its investigation. The agent learns from each step and adapts its approach.

### 2. Review Agent Actions

Pay attention to what the agent is doing, especially:
- File edits (review the changes)
- Terraform applies (check what resources are being created/modified)
- AWS CLI commands (understand what's being queried)

### 3. Verify the Solution

After the agent reports success:
- Test your application to confirm it's working
- Check AWS console to verify resources are healthy
- Review any configuration changes the agent made

### 4. Learn from the Agent

The agent's debugging process is educational. Watch how it:
- Systematically investigates issues
- Correlates symptoms with root causes
- Applies fixes incrementally
- Verifies each change

You can learn debugging techniques from observing the agent.

## Safety and Limitations

### What the Agent Can Do

✅ **Safe Actions:**
- Read AWS resource status
- Search configuration files
- Run terraform plan
- Edit configuration files
- Create missing resources
- Update resource configurations

### What the Agent Won't Do

❌ **Dangerous Actions (Protected):**
- Delete production resources without clear justification
- Modify resources in the wrong environment
- Make changes outside the working directory
- Bypass security controls

### Safety Mechanisms

1. **Iteration Limit**: Agent stops after 20 iterations (prevents infinite loops)
2. **Command Timeouts**: Each action has a timeout (2-15 minutes)
3. **User Cancellation**: Press 'q' to stop at any time
4. **Environment Isolation**: Agent works in the selected environment only
5. **Audit Trail**: All actions are recorded in the TUI

### When the Agent Can't Help

The agent may not be able to fix:
- Problems requiring manual AWS console actions
- Issues with external services (GitHub, third-party APIs)
- Hardware or network infrastructure problems
- Permission issues with AWS credentials
- Problems outside the scope of infrastructure (application bugs)

In these cases, the agent will report what it found and suggest manual steps.

## Troubleshooting the Agent

### Agent Won't Start

**Problem:** "ANTHROPIC_API_KEY not set"

**Solution:**
```bash
export ANTHROPIC_API_KEY=your_key_here
```

### Agent Keeps Failing

**Problem:** Agent reaches max iterations without solving

**Possible Causes:**
- Problem is outside agent's scope
- AWS permissions are insufficient
- External dependency issue

**Next Steps:**
- Review the agent's investigation history
- Manually check the resources the agent examined
- Consult the error messages for clues

### Agent Made Wrong Changes

**Problem:** Agent edited configuration incorrectly

**Solution:**
1. Check git diff to see what changed:
   ```bash
   git diff env/dev/main.tf
   ```

2. Revert if needed:
   ```bash
   git checkout env/dev/main.tf
   ```

3. Review the YAML configuration backups:
   ```bash
   ls -la project/*.backup*
   ```

### Agent is Slow

**Problem:** Agent takes a long time between iterations

**Causes:**
- LLM API latency (30s timeout per call)
- Long-running AWS commands
- Large terraform applies

**This is Normal:** The agent prioritizes accuracy over speed.

## Advanced Usage

### Running Agent from Command Line

(Future feature - not yet implemented)

```bash
# Run agent with specific problem
./meroku agent "ECS service not starting"

# Run agent with error file
./meroku agent --error-file terraform-error.log

# Run agent in non-interactive mode
./meroku agent --auto --problem "Fix deployment"
```

### Agent Configuration

(Future feature - not yet implemented)

Customize agent behavior in `agent.yaml`:
```yaml
max_iterations: 20
auto_approve_terraform: true
verbose_logging: false
allowed_actions:
  - aws_cli
  - shell
  - file_edit
  - terraform_plan
  - terraform_apply
```

### Integration with CI/CD

(Future feature - not yet implemented)

The agent could be integrated into CI/CD pipelines to automatically fix deployment failures.

## Getting Help

### Agent Logs

All agent actions are displayed in the TUI. To review:
- Use ↑/↓ to scroll through iterations
- Each iteration shows full command and output

### Reporting Issues

If the agent behaves unexpectedly:

1. **Note the context:**
   - What environment were you deploying?
   - What was the initial error?
   - What did the agent try to do?

2. **Check agent iterations:**
   - Review the thought process
   - Identify where it went wrong
   - Note any error messages

3. **File an issue:**
   - Include the agent's iteration history
   - Provide the initial error message
   - Describe the unexpected behavior

## Cost Considerations

Each agent run uses Claude API:
- Average: 10-20 iterations per problem
- Cost per iteration: ~$0.01-0.03
- Total per agent run: ~$0.10-0.50

This is typically much cheaper than:
- Developer time debugging manually (30-60 minutes)
- Trial-and-error terraform applies (AWS costs)
- Downtime from unresolved issues

## FAQ

**Q: Will the agent make changes without asking?**
A: Yes, that's the point. It's autonomous. However, you can stop it at any time with 'q', and all actions are shown in real-time.

**Q: Can I undo agent changes?**
A: Yes. Use git to revert file changes. For AWS resources, run `terraform destroy` or manually delete via console.

**Q: How does the agent know what to do?**
A: It uses Claude (Anthropic's AI) to reason about the problem. Each iteration, it thinks about what to investigate next based on what it learned from previous steps.

**Q: Is my data sent to Anthropic?**
A: Yes, the agent sends error messages and command outputs to Claude API for analysis. Don't use the agent if your logs contain sensitive data.

**Q: Can I use a different LLM?**
A: Currently, only Claude Sonnet 4.5 is supported. Future versions may support other models.

**Q: What if the agent breaks something?**
A: All infrastructure is managed by terraform, so you can always restore by reverting config files and running terraform apply. Additionally, the agent is designed to be cautious with destructive actions.

## Conclusion

The AI Agent is a powerful tool for autonomous infrastructure troubleshooting. By letting the agent investigate and fix issues, you can:

- Save time debugging complex problems
- Learn from the agent's systematic approach
- Reduce deployment downtime
- Focus on higher-level architectural decisions

Start with simple issues to get comfortable with how the agent works, then gradually use it for more complex problems as you build trust in its capabilities.

Happy troubleshooting! 🤖

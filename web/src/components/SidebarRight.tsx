import { Bot, Box, ChevronDown, ChevronRight, Copy, Database, Plus, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import type { BridgeVariable } from "../types/customTerraform";

// Define ModuleHelper interface locally for now
export interface ModuleHelper {
  name: string;
  source: string;
  inputs: string[];
}

export interface SidebarRightProps {
  variables: BridgeVariable[];
  modules?: ModuleHelper[];
  onInsert: (text: string) => void;
  environment?: string;
}

// Generate AI system prompt with all available variables
function generateAIPrompt(variables: BridgeVariable[], environment?: string): string {
  // Group variables by category
  const inputVars: BridgeVariable[] = [];
  const moduleVars: Record<string, BridgeVariable[]> = {};
  const dataVars: BridgeVariable[] = [];
  const otherVars: BridgeVariable[] = [];

  for (const v of variables) {
    if (v.name.startsWith("var.")) {
      inputVars.push(v);
    } else if (v.name.startsWith("module.")) {
      const parts = v.name.split(".");
      const moduleName = parts[1] || "unknown";
      if (!moduleVars[moduleName]) {
        moduleVars[moduleName] = [];
      }
      moduleVars[moduleName].push(v);
    } else if (v.name.startsWith("data.")) {
      dataVars.push(v);
    } else {
      otherVars.push(v);
    }
  }

  const formatVariable = (v: BridgeVariable) => {
    let line = `- \`${v.name}\` (${v.type})`;
    if (v.description) {
      line += `: ${v.description}`;
    }
    if (v.conditional) {
      line += ` [Conditional: ${v.conditional}]`;
    }
    return line;
  };

  let prompt = `# Custom Terraform Extension Assistant

You are an expert Terraform developer helping to create custom AWS infrastructure resources. The user is working with a managed infrastructure platform that provides pre-configured modules and variables.

## Context

${environment ? `- **Environment**: ${environment}` : ""}
- **Cloud Provider**: AWS
- **Terraform Version**: 1.5+
- **Purpose**: Extend the base infrastructure with custom resources

## Your Role

Help the user write Terraform code (.tf files) for custom resources that integrate with the existing infrastructure. The code will be placed in a custom extensions directory and automatically included during \`terraform apply\`.

## Available Variables

These variables are available from the base infrastructure and can be referenced in custom Terraform code:

`;

  // Input Variables
  if (inputVars.length > 0) {
    prompt += `### Input Variables (var.*)\nThese are configuration values passed to the infrastructure:\n\n`;
    for (const v of inputVars) {
      prompt += `${formatVariable(v)}\n`;
    }
    prompt += "\n";
  }

  // Module Outputs
  const moduleNames = Object.keys(moduleVars).sort();
  if (moduleNames.length > 0) {
    prompt += `### Module Outputs\nOutputs from infrastructure modules that can be referenced:\n\n`;
    for (const moduleName of moduleNames) {
      prompt += `#### Module: ${moduleName}\n`;
      for (const v of moduleVars[moduleName]) {
        prompt += `${formatVariable(v)}\n`;
      }
      prompt += "\n";
    }
  }

  // Data Sources
  if (dataVars.length > 0) {
    prompt += `### Data Sources (data.*)\nData sources providing information about existing resources:\n\n`;
    for (const v of dataVars) {
      prompt += `${formatVariable(v)}\n`;
    }
    prompt += "\n";
  }

  // Other Variables
  if (otherVars.length > 0) {
    prompt += `### Other Available References\n\n`;
    for (const v of otherVars) {
      prompt += `${formatVariable(v)}\n`;
    }
    prompt += "\n";
  }

  prompt += `## Guidelines

1. **Use Available Variables**: Reference the variables listed above instead of hardcoding values
2. **Naming Convention**: Use \`\${var.project}-\${var.env}-\` prefix for resource names
3. **Tags**: Include standard tags: \`Project = var.project\`, \`Environment = var.env\`
4. **Security**: Follow AWS security best practices (least privilege IAM, encryption at rest)
5. **Dependencies**: Use \`depends_on\` when resources depend on module outputs
6. **Outputs**: Define outputs for any values that other resources might need

## Example Usage

\`\`\`hcl
# Example: Create an S3 bucket using available variables
resource "aws_s3_bucket" "custom_bucket" {
  bucket = "\${var.project}-\${var.env}-custom-data"

  tags = {
    Name        = "\${var.project}-\${var.env}-custom-data"
    Project     = var.project
    Environment = var.env
  }
}

# Example: Reference VPC from base infrastructure
resource "aws_security_group" "custom_sg" {
  name        = "\${var.project}-\${var.env}-custom-sg"
  description = "Custom security group"
  vpc_id      = module.vpc.vpc_id  # Reference VPC module output

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [module.vpc.vpc_cidr]
  }
}
\`\`\`

## Instructions

When the user describes what they need, provide complete, working Terraform code that:
1. Integrates with the existing infrastructure using the available variables
2. Follows Terraform and AWS best practices
3. Includes appropriate comments explaining the configuration
4. Defines any necessary outputs

Ask clarifying questions if the requirements are unclear.
`;

  return prompt;
}

export function SidebarRight({ variables, modules = [], onInsert, environment }: SidebarRightProps) {
  const [activeTab, setActiveTab] = useState<"vars" | "modules">("vars");
  const [filterText, setFilterText] = useState("");
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({
    "Input Variables": true,
    "Data Sources": true,
  });

  const handleCopyAIPrompt = () => {
    const prompt = generateAIPrompt(variables, environment);
    navigator.clipboard.writeText(prompt).then(() => {
      toast.success("AI prompt copied to clipboard", {
        description: "Paste it into ChatGPT, Claude, or any AI assistant",
      });
    }).catch((err) => {
      console.error("Failed to copy:", err);
      toast.error("Failed to copy to clipboard");
    });
  };

  // Group variables by their prefix (var., module.name., data.)
  const groupedVariables = useMemo(() => {
    const groups: Record<string, BridgeVariable[]> = {};

    variables.forEach((v) => {
      // Apply filter
      if (
        filterText &&
        !v.name.toLowerCase().includes(filterText.toLowerCase()) &&
        !v.description.toLowerCase().includes(filterText.toLowerCase())
      ) {
        return;
      }

      let groupName = "Other";
      if (v.name.startsWith("var.")) {
        groupName = "Input Variables";
      } else if (v.name.startsWith("module.")) {
        // Extract module name: module.vpc.id -> vpc
        const parts = v.name.split(".");
        if (parts.length > 1) {
          groupName = `Module: ${parts[1]}`;
        }
      } else if (v.name.startsWith("data.")) {
        groupName = "Data Sources";
      }

      if (!groups[groupName]) {
        groups[groupName] = [];
      }
      groups[groupName].push(v);
    });

    // Auto-expand groups if searching
    if (filterText) {
      const allOpen: Record<string, boolean> = {};
      Object.keys(groups).forEach((k) => (allOpen[k] = true));
      return { groups, isSearching: true };
    }

    return { groups, isSearching: false };
  }, [variables, filterText]);

  const toggleGroup = (group: string) => {
    setExpandedGroups((prev) => ({
      ...prev,
      [group]: !prev[group],
    }));
  };

  const currentGroups = groupedVariables.groups;

  return (
    <div className="flex flex-col h-full bg-gray-950 border-l border-gray-800 w-80 flex-shrink-0">
      {/* AI Prompt Button */}
      <div className="p-3 border-b border-gray-800 bg-gray-900/50 flex-shrink-0">
        <button
          onClick={handleCopyAIPrompt}
          className="w-full flex items-center justify-center gap-2 px-3 py-2.5 bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-500 hover:to-blue-500 text-white text-sm font-medium rounded-lg transition-all shadow-lg shadow-purple-900/20 hover:shadow-purple-900/40"
        >
          <Bot size={16} />
          Copy AI Prompt
        </button>
        <p className="text-[10px] text-gray-500 text-center mt-2">
          Get a system prompt with all variables for ChatGPT, Claude, etc.
        </p>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-gray-800 bg-gray-900/50 flex-shrink-0">
        <button
          onClick={() => setActiveTab("vars")}
          className={`flex-1 py-3 text-xs font-medium tracking-wide transition-colors
            ${
              activeTab === "vars"
                ? "text-blue-400 border-b-2 border-blue-500 bg-gray-900"
                : "text-gray-500 hover:text-gray-300"
            }`}
        >
          VARIABLES
        </button>
        <button
          onClick={() => setActiveTab("modules")}
          className={`flex-1 py-3 text-xs font-medium tracking-wide transition-colors
            ${
              activeTab === "modules"
                ? "text-purple-400 border-b-2 border-purple-500 bg-gray-900"
                : "text-gray-500 hover:text-gray-300"
            }`}
        >
          HELPERS
        </button>
      </div>

      <div className="flex-1 overflow-y-auto bg-gray-950 flex flex-col">
        {activeTab === "vars" && (
          <>
            {/* Search Filter */}
            <div className="p-3 border-b border-gray-800 bg-gray-900/30 sticky top-0 z-10 backdrop-blur-sm">
              <div className="relative group">
                <Search
                  className="absolute left-2.5 top-2 text-gray-500 group-focus-within:text-blue-400 transition-colors"
                  size={14}
                />
                <input
                  type="text"
                  value={filterText}
                  onChange={(e) => setFilterText(e.target.value)}
                  placeholder="Filter variables..."
                  className="w-full bg-gray-900 border border-gray-700 rounded py-1.5 pl-8 pr-3 text-xs text-gray-200 focus:outline-none focus:border-blue-500 placeholder-gray-600 transition-all"
                />
                {filterText && (
                  <button
                    onClick={() => setFilterText("")}
                    className="absolute right-2 top-2 text-gray-600 hover:text-gray-300"
                  >
                    <div className="rotate-45">
                        <Plus size={14} />
                    </div>
                  </button>
                )}
              </div>
            </div>

            {/* Variable Groups */}
            <div className="flex-1 pb-4">
              {Object.keys(currentGroups).length === 0 ? (
                <div className="p-8 text-center text-gray-500 text-xs">
                  No variables found.
                </div>
              ) : (
                (Object.entries(currentGroups) as [string, BridgeVariable[]][])
                  .sort()
                  .map(([groupName, vars]) => {
                    const isOpen =
                      groupedVariables.isSearching || expandedGroups[groupName];

                    return (
                      <div key={groupName} className="border-b border-gray-800/50">
                        <button
                          onClick={() => toggleGroup(groupName)}
                          className="w-full flex items-center px-3 py-2 bg-gray-900/20 hover:bg-gray-800/80 transition-colors text-left"
                        >
                          <span className="text-gray-500 mr-2">
                            {isOpen ? (
                              <ChevronDown size={14} />
                            ) : (
                              <ChevronRight size={14} />
                            )}
                          </span>
                          <span className="text-xs font-semibold text-gray-400 flex-1">
                            {groupName}
                          </span>
                          <span className="text-[10px] bg-gray-800 text-gray-500 px-1.5 py-0.5 rounded-full min-w-[20px] text-center">
                            {vars.length}
                          </span>
                        </button>

                        {isOpen && (
                          <div className="bg-gray-950">
                            {vars.map((v) => (
                              <div
                                key={v.name}
                                className="group flex items-start justify-between px-4 py-2 hover:bg-gray-900 border-l-2 border-transparent hover:border-blue-500 transition-all"
                              >
                                <div className="flex-1 min-w-0 mr-3">
                                  <div className="flex items-center gap-2 mb-0.5">
                                    <code
                                      className="text-xs font-mono text-blue-300 truncate cursor-pointer hover:underline decoration-blue-500/50"
                                      onClick={() => onInsert(`${v.name}`)}
                                      title="Click to insert"
                                    >
                                      {v.name}
                                    </code>
                                  </div>
                                  <div className="flex items-center gap-2">
                                    <span className="text-[10px] text-gray-500 font-mono px-1 rounded bg-gray-900/50 border border-gray-800">
                                      {v.type}
                                    </span>
                                  </div>
                                  <p className="text-[10px] text-gray-600 mt-1 truncate group-hover:text-gray-500 group-hover:whitespace-normal transition-colors">
                                    {v.description}
                                  </p>
                                </div>
                                <button
                                  onClick={() => onInsert(`${v.name}`)}
                                  className="opacity-0 group-hover:opacity-100 p-1.5 text-gray-400 hover:text-white hover:bg-gray-800 rounded transition-all flex-shrink-0"
                                  title="Insert Variable"
                                >
                                  <Plus size={14} />
                                </button>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    );
                  })
              )}
            </div>
          </>
        )}

        {activeTab === "modules" && (
          <div className="p-4 space-y-6">
            <div className="space-y-4">
              <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">
                Shared Modules
              </div>
              {modules.length === 0 && (
                  <div className="text-xs text-gray-600 italic">No modules available</div>
              )}
              {modules.map((m) => (
                <div
                  key={m.name}
                  className="group p-3 rounded bg-gray-900 border border-gray-800 hover:border-purple-900/50 transition-all"
                >
                  <div className="flex justify-between items-center mb-2">
                    <div className="flex items-center gap-2">
                      <Box size={14} className="text-purple-400" />
                      <span className="font-semibold text-sm text-gray-200">
                        {m.name}
                      </span>
                    </div>
                    <button
                      onClick={() =>
                        onInsert(
                          `module "${m.name}" {\n  source = "${m.source}"\n${m.inputs
                            .map((i) => `  ${i} = "..."`)
                            .join("\n")}\n}`
                        )
                      }
                      className="opacity-0 group-hover:opacity-100 p-1 text-gray-400 hover:text-white transition-opacity"
                      title="Insert Module Snippet"
                    >
                      <Copy size={14} />
                    </button>
                  </div>
                  <div className="text-xs text-gray-500 font-mono mb-2 bg-gray-950/50 p-1 rounded">
                    source = "{m.source}"
                  </div>
                  <div className="text-xs text-gray-500">
                    <span className="block mb-1 font-medium text-gray-400">
                      Inputs:
                    </span>
                    <div className="flex flex-wrap gap-1">
                      {m.inputs.map((input) => (
                        <span
                          key={input}
                          className="px-1.5 py-0.5 bg-gray-800 rounded border border-gray-700 text-gray-400"
                        >
                          {input}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              ))}

              <div className="mt-8 p-4 bg-gray-900/30 rounded border border-dashed border-gray-800 text-center">
                <Database
                  size={24}
                  className="mx-auto text-gray-600 mb-2"
                />
                <p className="text-xs text-gray-500">
                  Resource definitions from <code>aws_provider</code> are
                  available in auto-complete.
                </p>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

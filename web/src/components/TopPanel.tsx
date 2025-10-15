import { AWS_REGIONS } from "../types/config";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";

interface TopPanelProps {
  selectedEnvironment: string | null;
  config: YamlInfrastructureConfig | null;
  activeEnvironmentProfile: string | null;
  activeEnvironmentAccountId: string | null;
  onConfigChange: (updates: Partial<YamlInfrastructureConfig>) => void;
}

export function TopPanel({
  selectedEnvironment,
  config,
  activeEnvironmentProfile,
  activeEnvironmentAccountId,
  onConfigChange,
}: TopPanelProps) {
  if (!selectedEnvironment) return null;

  return (
    <div className="absolute top-4 left-1/2 -translate-x-1/2 z-10 px-4 w-[calc(100%-8rem)]">
      <div className="bg-gray-800/95 backdrop-blur-sm rounded-lg border border-gray-700 shadow-lg w-fit mx-auto">
        <div className="flex flex-wrap items-center justify-center gap-x-3 gap-y-1.5 px-3 py-2 xl:px-4">
          {/* Environment */}
          <div className="flex items-center gap-2 flex-shrink-0 whitespace-nowrap">
            <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
            <span className="text-xs text-gray-400 uppercase tracking-wide sm:inline hidden">
              Environment
            </span>
            <span className="text-xs text-gray-400 uppercase tracking-wide sm:hidden">
              Env
            </span>
            <span className="text-sm font-semibold text-white">
              {selectedEnvironment}
            </span>
          </div>

          {config && (
            <>
              <div className="h-5 w-px bg-gray-600 flex-shrink-0 hidden xl:block" />

              {/* Project */}
              <div className="flex items-center gap-2 flex-shrink-0 whitespace-nowrap">
                <span className="text-xs text-gray-400 uppercase tracking-wide sm:inline hidden">
                  Project
                </span>
                <span className="text-xs text-gray-400 sm:hidden">Proj:</span>
                <span
                  className="text-sm font-semibold text-white sm:inline hidden"
                  title={config.project}
                >
                  {config.project}
                </span>
                <span
                  className="text-xs font-semibold text-white truncate max-w-[80px] sm:hidden"
                  title={config.project}
                >
                  {config.project}
                </span>
              </div>

              <div className="h-5 w-px bg-gray-600 flex-shrink-0 hidden xl:block" />

              {/* Region */}
              <div className="flex items-center gap-2 flex-shrink-0 whitespace-nowrap">
                <span className="text-xs text-gray-400 uppercase tracking-wide sm:inline hidden">
                  Region
                </span>
                <span className="text-xs text-gray-400 sm:hidden">Reg:</span>
                <Select
                  value={config.region}
                  onValueChange={(value: string) => {
                    onConfigChange({ region: value });
                  }}
                >
                  <SelectTrigger className="h-7 xl:w-[200px] sm:w-[180px] w-[140px] sm:text-xs text-[10px] font-mono bg-gray-800/50 border-gray-600 hover:bg-gray-700/50 sm:h-7 h-6">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {AWS_REGIONS.map((region) => (
                      <SelectItem key={region.value} value={region.value}>
                        {region.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {activeEnvironmentProfile && activeEnvironmentAccountId && (
                <>
                  <div className="h-5 w-px bg-gray-600 flex-shrink-0 hidden xl:block" />

                  {/* AWS Profile */}
                  <div className="flex items-center gap-2 flex-shrink-0 whitespace-nowrap">
                    <span className="text-xs text-gray-400 uppercase tracking-wide sm:inline hidden">
                      Profile
                    </span>
                    <span className="text-xs text-gray-400 sm:hidden">
                      Prof:
                    </span>
                    <span
                      className="sm:text-sm text-xs font-semibold text-white truncate sm:max-w-[120px] max-w-[80px]"
                      title={activeEnvironmentProfile}
                    >
                      {activeEnvironmentProfile}
                    </span>
                  </div>

                  <div className="h-5 w-px bg-gray-600 flex-shrink-0 hidden xl:block" />

                  {/* Account ID */}
                  <div className="flex items-center gap-2 flex-shrink-0 whitespace-nowrap">
                    <span className="text-xs text-gray-400 uppercase tracking-wide sm:inline hidden">
                      Account
                    </span>
                    <span className="text-xs text-gray-400 sm:hidden">
                      Acct:
                    </span>
                    <span
                      className="sm:text-sm text-xs font-semibold text-white font-mono sm:max-w-none max-w-[100px] truncate"
                      title={activeEnvironmentAccountId}
                    >
                      {activeEnvironmentAccountId}
                    </span>
                  </div>
                </>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

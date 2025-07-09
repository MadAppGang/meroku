import type { Plugin } from 'vite';

export function stripUseClient(): Plugin {
  return {
    name: 'strip-use-client',
    transform(code, id) {
      if (id.includes('node_modules')) {
        return null;
      }
      
      // Remove "use client" directive from the beginning of files
      if (code.startsWith('"use client"') || code.startsWith("'use client'")) {
        const lines = code.split('\n');
        lines[0] = ''; // Remove the first line
        return {
          code: lines.join('\n'),
          map: null,
        };
      }
      
      return null;
    },
  };
}
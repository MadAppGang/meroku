import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";

const __dirname = fileURLToPath(new URL(".", import.meta.url));

// https://vite.dev/config/
export default defineConfig({
	plugins: [react()],
	server: {
		proxy: {
			"/api": {
				target: "http://localhost:8080",
				changeOrigin: true,
				secure: false,
			},
		},
	},
	resolve: {
		extensions: [".mjs", ".js", ".mts", ".ts", ".jsx", ".tsx", ".json"],
		alias: {
			"@": resolve(__dirname, "./src"),
			"@components": resolve(__dirname, "./src/components"),
			"@ui": resolve(__dirname, "./src/components/ui"),
		},
		// Explicitly tell Vite to look for index files
		mainFields: ["module", "jsnext:main", "jsnext", "main"],
	},
	build: {
		outDir: resolve(__dirname, "../app/webapp"),
		emptyOutDir: true,
		commonjsOptions: {
			transformMixedEsModules: true,
		},
		rollupOptions: {
			onwarn(warning, warn) {
				// Suppress "use client" directive warnings
				if (warning.code === "MODULE_LEVEL_DIRECTIVE") {
					return;
				}
				warn(warning);
			},
		},
	},
	optimizeDeps: {
		include: ["react", "react-dom"],
		esbuildOptions: {
			target: "es2020",
		},
	},
});

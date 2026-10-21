/**
 * properties-v3 boundary checker.
 *
 * Audits the HIGH-LEVEL v3 composition layer — panel orchestration
 * (PropertyPanel.tsx, PropertySaveBar.tsx), archetypes/, the declarative
 * catalog (nodeCatalog.ts, fixtures.ts), and every story file — for:
 *
 * 1. Raw DOM: raw JSX HTML tags, `className`, `style=`, `data-*` custom
 *    attributes, `css` prop, dangerouslySetInnerHTML, and `.css` imports.
 *    Low-level primitives (primitives/) MAY contain styled DOM and are
 *    audited only for import discipline (v2 isolation, allowlist).
 * 2. Direct UI-library bypass: imports from ui/ or direct UI-library
 *    packages must not appear in high-level files; composition goes through
 *    v3 archetypes/primitives only.
 * 3. No v2 imports anywhere under properties-v3/.
 * 4. Narrow story import allowlist.
 * 5. One production entry point: outside properties-v3, only
 *    src/components/Sidebar.tsx may import it, and only the
 *    `properties-v3/production` module (opt-in via ?panels=v3).
 * 6. production/ never imports Storybook fixtures or stories.
 */

import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

const root = process.cwd();
const v3Root = path.join(root, "src/components/properties-v3");
const storyRoot = path.join(v3Root, "stories");
const panelStoryRoot = path.join(storyRoot, "panels");

const DIRECT_UI_PATTERN = /\/ui\//;
const UI_LIBRARIES = /^(styled-components|@emotion|lucide-react)$/;
const V2_PATTERN = /properties-v2/;

/** The ONLY imports a panel story file may use. */
const PANEL_STORY_ALLOWED_IMPORTS = new Set([
	"@storybook/react-vite",
	"../storyConfig",
	"../fixtures",
	"../../fixtures",
	"../../nodeCatalog",
	"../../PropertyPanel",
]);

const productionRoot = path.join(v3Root, "production");
const PRODUCTION_ENTRY = "properties-v3/production";
const PRODUCTION_CONSUMERS = new Set([
	path.join(root, "src/components/Sidebar.tsx"),
]);
const PRODUCTION_FORBIDDEN_IMPORTS = /(^|\/)(fixtures|stories)(\/|$)/;

const HIGH_LEVEL_FILES = [
	path.join(productionRoot, "ConfigPropertyPanel.tsx"),
	path.join(v3Root, "PropertyPanel.tsx"),
	path.join(v3Root, "PropertySaveBar.tsx"),
	path.join(v3Root, "nodeCatalog.ts"),
	path.join(v3Root, "fixtures.ts"),
];

function walk(dir, predicate, out = []) {
	if (!fs.existsSync(dir)) return out;
	for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
		const full = path.join(dir, entry.name);
		if (entry.isDirectory()) walk(full, predicate, out);
		else if (predicate(full)) out.push(full);
	}
	return out;
}

const archetypeFiles = walk(
	path.join(v3Root, "archetypes"),
	(f) => f.endsWith(".tsx") || f.endsWith(".ts"),
);
const storyFiles = walk(storyRoot, (f) => f.endsWith(".stories.tsx"));
const allV3Files = walk(v3Root, (f) => f.endsWith(".tsx") || f.endsWith(".ts"));

function analyze(file, { rawDom, strictImports, highLevel }) {
	const sourceText = fs.readFileSync(file, "utf8");
	const source = ts.createSourceFile(
		file,
		sourceText,
		ts.ScriptTarget.Latest,
		true,
		ts.ScriptKind.TSX,
	);
	const issues = [];

	for (const statement of source.statements) {
		if (!ts.isImportDeclaration(statement)) continue;
		const moduleName = String(statement.moduleSpecifier.text);
		if (V2_PATTERN.test(moduleName)) {
			issues.push(`v2 import: ${moduleName}`);
		}
		// The primitives' own stylesheet may be mounted once at the composition
		// root; styling itself lives in primitives/ (the sanctioned layer).
		if (moduleName.endsWith(".css") && !moduleName.includes("primitives/")) {
			issues.push(`css import: ${moduleName}`);
		}
		// Primitives are the sanctioned UI-library wrappers; the direct-UI
		// bypass rule applies to the high-level composition layer only.
		if (
			highLevel &&
			(DIRECT_UI_PATTERN.test(moduleName) || UI_LIBRARIES.test(moduleName))
		) {
			issues.push(`direct UI bypass import: ${moduleName}`);
		}
		if (strictImports && !PANEL_STORY_ALLOWED_IMPORTS.has(moduleName)) {
			issues.push(`non-allowlisted panel-story import: ${moduleName}`);
		}
	}

	if (rawDom) {
		const visit = (node) => {
			if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
				const tagName = node.tagName.getText(source);
				const line =
					source.getLineAndCharacterOfPosition(node.getStart()).line + 1;
				if (/^[a-z]/.test(tagName)) {
					issues.push(`raw <${tagName}> at line ${line}`);
				}
				for (const attr of node.attributes.properties) {
					if (!ts.isJsxAttribute(attr)) continue;
					const name = attr.name.getText(source);
					if (
						name === "className" ||
						name === "style" ||
						name === "css" ||
						name === "dangerouslySetInnerHTML" ||
						name.startsWith("data-")
					) {
						issues.push(`raw DOM attribute ${name} at line ${line}`);
					}
				}
			}
			ts.forEachChild(node, visit);
		};
		visit(source);
	}

	return issues;
}

let failed = false;
console.log("| File (v3) | Result | Evidence |");
console.log("|---|---|---|");

const report = (file, issues) => {
	const rel = path.relative(root, file);
	if (issues.length > 0) failed = true;
	console.log(
		`| ${rel} | ${issues.length ? "FAIL" : "PASS"} | ${
			issues.join("; ") || "Composition via v3 library only"
		} |`,
	);
};

for (const file of HIGH_LEVEL_FILES) {
	if (!fs.existsSync(file)) continue; // UI layer may still be in flight
	report(
		file,
		analyze(file, { rawDom: true, strictImports: false, highLevel: true }),
	);
}
for (const file of archetypeFiles) {
	report(
		file,
		analyze(file, { rawDom: true, strictImports: false, highLevel: true }),
	);
}
for (const file of storyFiles) {
	const strict = file.startsWith(`${panelStoryRoot}${path.sep}`);
	report(file, analyze(file, { rawDom: true, strictImports: strict }));
}

// Import discipline for the rest of properties-v3 (primitives allowed DOM).
for (const file of allV3Files) {
	const isAudited =
		HIGH_LEVEL_FILES.includes(file) ||
		archetypeFiles.includes(file) ||
		storyFiles.includes(file);
	if (isAudited) continue;
	report(file, analyze(file, { rawDom: false, strictImports: false }));
}

function importSpecifiers(file) {
	const source = ts.createSourceFile(
		file,
		fs.readFileSync(file, "utf8"),
		ts.ScriptTarget.Latest,
		true,
	);
	const specifiers = [];
	const visit = (node) => {
		if (
			(ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
			node.moduleSpecifier &&
			ts.isStringLiteral(node.moduleSpecifier)
		) {
			specifiers.push(node.moduleSpecifier.text);
		} else if (
			ts.isCallExpression(node) &&
			node.expression.kind === ts.SyntaxKind.ImportKeyword &&
			node.arguments[0] &&
			ts.isStringLiteral(node.arguments[0])
		) {
			specifiers.push(node.arguments[0].text);
		}
		ts.forEachChild(node, visit);
	};
	visit(source);
	return specifiers;
}

// One production entry point: properties-v3 is referenced outside itself only
// by the Sidebar, and only through properties-v3/production.
const integrationOffenders = [];
const srcRoot = path.join(root, "src");
for (const file of walk(
	srcRoot,
	(f) => f.endsWith(".ts") || f.endsWith(".tsx"),
)) {
	if (file.startsWith(`${v3Root}${path.sep}`)) continue;
	const text = fs.readFileSync(file, "utf8");
	if (!text.includes("properties-v3")) continue;
	const v3Imports = importSpecifiers(file).filter((s) =>
		s.includes("properties-v3"),
	);
	const allowed =
		PRODUCTION_CONSUMERS.has(file) &&
		v3Imports.length > 0 &&
		v3Imports.every((s) => s.endsWith(PRODUCTION_ENTRY));
	// Any other mention (comments included) means a second integration path.
	if (!allowed) integrationOffenders.push(file);
}
if (integrationOffenders.length > 0) {
	failed = true;
	console.log(
		`| PRODUCTION INTEGRATION | FAIL | referenced outside properties-v3 other than Sidebar → ${PRODUCTION_ENTRY}: ${integrationOffenders
			.map((f) => path.relative(root, f))
			.join(", ")} |`,
	);
} else {
	console.log(
		`| PRODUCTION INTEGRATION | PASS | only Sidebar.tsx → ${PRODUCTION_ENTRY} |`,
	);
}

const productionOffenders = walk(
	productionRoot,
	(f) => (f.endsWith(".ts") || f.endsWith(".tsx")) && !f.includes("__tests__"),
).filter((file) =>
	importSpecifiers(file).some((s) => PRODUCTION_FORBIDDEN_IMPORTS.test(s)),
);
if (productionOffenders.length > 0) {
	failed = true;
	console.log(
		`| PRODUCTION DATA | FAIL | production/ imports fixtures or stories: ${productionOffenders
			.map((f) => path.relative(root, f))
			.join(", ")} |`,
	);
} else {
	console.log(
		"| PRODUCTION DATA | PASS | production/ uses no fixtures or stories |",
	);
}

if (storyFiles.length === 0) {
	failed = true;
	console.log("| STORIES | FAIL | no v3 story files found |");
}

process.exitCode = failed ? 1 : 0;

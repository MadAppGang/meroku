import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

const propertyRoot = path.join(process.cwd(), "src/components/properties");
const panelStoryRoot = path.join(propertyRoot, "stories/panels");
const panelStories = fs
	.readdirSync(panelStoryRoot)
	.filter((file) => file.endsWith(".stories.tsx"))
	.sort()
	.map((file) => path.join(panelStoryRoot, file));
const auditedFiles = [
	...panelStories,
	path.join(propertyRoot, "PropertyPanel.tsx"),
	path.join(propertyRoot, "ServiceCompactGrouped.tsx"),
	path.join(propertyRoot, "NodePanelStoryFixtures.stories.tsx"),
	path.join(propertyRoot, "ServicePanelStoryFixtures.stories.tsx"),
];

function analyze(file) {
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
		if (moduleName.endsWith(".css")) {
			issues.push(`panel-local stylesheet: ${moduleName}`);
		}
		if (/\/ui\//.test(moduleName)) {
			issues.push(`direct UI import: ${moduleName}`);
		}
	}

	function visit(node) {
		if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
			const tagName = node.tagName.getText(source);
			const line =
				source.getLineAndCharacterOfPosition(node.getStart()).line + 1;
			if (/^[a-z]/.test(tagName)) {
				issues.push(`raw <${tagName}> at line ${line}`);
			}

			for (const attribute of node.attributes.properties) {
				if (!ts.isJsxAttribute(attribute)) continue;
				const name = attribute.name.getText(source);
				if (
					name === "className" ||
					name === "style" ||
					name.startsWith("data-")
				) {
					issues.push(`custom ${name} at line ${line}`);
				}
			}
		}
		ts.forEachChild(node, visit);
	}

	visit(source);
	return issues;
}

let failed = false;
console.log("| File | Result | Evidence |");
console.log("|---|---|---|");
for (const file of auditedFiles) {
	const issues = analyze(file);
	if (issues.length > 0) failed = true;
	console.log(
		`| ${path.relative(process.cwd(), file)} | ${issues.length ? "FAIL" : "PASS"} | ${issues.join("; ") || "Library composition only"} |`,
	);
}

if (failed) process.exitCode = 1;

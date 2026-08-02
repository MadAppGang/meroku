import Editor, { type Monaco } from "@monaco-editor/react";
import type * as monacoEditor from "monaco-editor";
import { useEffect, useRef } from "react";
import type { BridgeVariable } from "../types/customTerraform";

interface TerraformEditorProps {
	value: string;
	onChange: (value: string) => void;
	bridgeVariables: BridgeVariable[];
	readOnly?: boolean;
}

export function TerraformEditor({
	value,
	onChange,
	bridgeVariables,
	readOnly = false,
}: TerraformEditorProps) {
	const editorRef = useRef<monacoEditor.editor.IStandaloneCodeEditor | null>(
		null,
	);
	const monacoRef = useRef<Monaco | null>(null);
	const bridgeVariablesRef = useRef(bridgeVariables);

	useEffect(() => {
		bridgeVariablesRef.current = bridgeVariables;
	}, [bridgeVariables]);

	const handleEditorDidMount = (
		editor: monacoEditor.editor.IStandaloneCodeEditor,
		monaco: Monaco,
	) => {
		editorRef.current = editor;
		monacoRef.current = monaco;

		// Register HCL language (Terraform)
		monaco.languages.register({ id: "hcl" });

		// Set HCL syntax highlighting
		monaco.languages.setMonarchTokensProvider("hcl", {
			defaultToken: "",
			tokenPostfix: ".hcl",

			keywords: [
				"resource",
				"data",
				"variable",
				"output",
				"locals",
				"module",
				"provider",
				"terraform",
				"backend",
				"true",
				"false",
				"null",
				"var",
				"local",
				"module",
				"path",
				"for",
				"in",
				"if",
				"else",
				"endif",
				"for_each",
				"count",
				"depends_on",
				"lifecycle",
				"dynamic",
			],

			typeKeywords: [
				"string",
				"number",
				"bool",
				"list",
				"map",
				"set",
				"object",
				"tuple",
				"any",
			],

			operators: [
				"=",
				">",
				"<",
				"!",
				"~",
				"?",
				":",
				"==",
				"<=",
				">=",
				"!=",
				"&&",
				"||",
				"++",
				"--",
				"+",
				"-",
				"*",
				"/",
				"&",
				"|",
				"^",
				"%",
				"<<",
				">>",
				">>>",
				"+=",
				"-=",
				"*=",
				"/=",
				"&=",
				"|=",
				"^=",
				"%=",
				"<<=",
				">>=",
				">>>=",
			],

			symbols: /[=><!~?:&|+\-*/^%]+/,

			escapes:
				/\\(?:[abfnrtv\\"']|x[0-9A-Fa-f]{1,4}|u[0-9A-Fa-f]{4}|U[0-9A-Fa-f]{8})/,

			tokenizer: {
				root: [
					// identifiers and keywords
					[
						/[a-z_$][\w$]*/,
						{
							cases: {
								"@typeKeywords": "type.identifier",
								"@keywords": "keyword",
								"@default": "identifier",
							},
						},
					],
					[/[A-Z][\w$]*/, "type.identifier"],

					// whitespace
					{ include: "@whitespace" },

					// delimiters and operators
					[/[{}()[\]]/, "@brackets"],
					[/[<>](?!@symbols)/, "@brackets"],
					[
						/@symbols/,
						{
							cases: {
								"@operators": "operator",
								"@default": "",
							},
						},
					],

					// numbers
					[/\d*\.\d+([eE][-+]?\d+)?/, "number.float"],
					[/0[xX][0-9a-fA-F]+/, "number.hex"],
					[/\d+/, "number"],

					// delimiter: after number because of .\d floats
					[/[;,.]/, "delimiter"],

					// strings
					[/"([^"\\]|\\.)*$/, "string.invalid"], // non-terminated string
					[/"/, { token: "string.quote", bracket: "@open", next: "@string" }],
				],

				comment: [
					[/[^/*]+/, "comment"],
					[/\/\*/, "comment", "@push"], // nested comment
					["\\*/", "comment", "@pop"],
					[/[/*]/, "comment"],
				],

				string: [
					[/[^\\"]+/, "string"],
					[/@escapes/, "string.escape"],
					[/\\./, "string.escape.invalid"],
					[/"/, { token: "string.quote", bracket: "@close", next: "@pop" }],
				],

				whitespace: [
					[/[ \t\r\n]+/, "white"],
					[/\/\*/, "comment", "@comment"],
					[/\/\/.*$/, "comment"],
					[/#.*$/, "comment"],
				],
			},
		});

		// Register completion provider for bridge variables and Terraform syntax
		monaco.languages.registerCompletionItemProvider("hcl", {
			triggerCharacters: [".", "{", "("],
			provideCompletionItems: (
				model: monacoEditor.editor.ITextModel,
				position: monacoEditor.Position,
			) => {
				const word = model.getWordUntilPosition(position);
				const range = {
					startLineNumber: position.lineNumber,
					endLineNumber: position.lineNumber,
					startColumn: word.startColumn,
					endColumn: word.endColumn,
				};

				const line = model.getLineContent(position.lineNumber);
				const textBeforeCursor = line.substring(0, position.column - 1);

				const suggestions: monacoEditor.languages.CompletionItem[] = [];

				// Bridge variables completion
				if (textBeforeCursor.includes("local.bridge")) {
					bridgeVariablesRef.current.forEach((variable) => {
						suggestions.push({
							label: variable.name,
							kind: monaco.languages.CompletionItemKind.Variable,
							detail: variable.type,
							documentation: variable.description,
							insertText: variable.name.replace("local.bridge.", ""),
							range: range,
						});
					});
				}

				// Module outputs completion
				if (textBeforeCursor.includes("module.")) {
					suggestions.push(
						{
							label: "module.workloads",
							kind: monaco.languages.CompletionItemKind.Module,
							documentation: "Main workloads module outputs",
							insertText: "workloads",
							range: range,
						},
						{
							label: "module.networking",
							kind: monaco.languages.CompletionItemKind.Module,
							documentation: "Networking module outputs",
							insertText: "networking",
							range: range,
						},
					);
				}

				// AWS resource types for custom extensions
				if (textBeforeCursor.match(/^\s*resource\s+"[\w-]*$/)) {
					const awsResources = [
						{
							label: "aws_sns_topic",
							detail: "SNS Topic for custom extensions",
							prefix: "ext_",
						},
						{
							label: "aws_sqs_queue",
							detail: "SQS Queue for custom extensions",
							prefix: "ext_",
						},
						{
							label: "aws_lambda_function",
							detail: "Lambda Function",
							prefix: "ext_",
						},
						{
							label: "aws_dynamodb_table",
							detail: "DynamoDB Table",
							prefix: "ext_",
						},
						{
							label: "aws_s3_bucket",
							detail: "S3 Bucket",
							prefix: "ext_",
						},
					];

					awsResources.forEach((resource) => {
						suggestions.push({
							label: resource.label,
							kind: monaco.languages.CompletionItemKind.Class,
							detail: resource.detail,
							documentation: `AWS resource: ${resource.detail}`,
							insertText: resource.label,
							range: range,
						});
					});
				}

				// Terraform keywords
				const keywords = [
					"resource",
					"data",
					"variable",
					"output",
					"locals",
					"module",
					"provider",
					"terraform",
				];

				keywords.forEach((keyword) => {
					if (keyword.startsWith(word.word.toLowerCase())) {
						suggestions.push({
							label: keyword,
							kind: monaco.languages.CompletionItemKind.Keyword,
							insertText: keyword,
							range: range,
						});
					}
				});

				return { suggestions };
			},
		});

		// Set editor theme
		monaco.editor.defineTheme("terraform-dark", {
			base: "vs-dark",
			inherit: true,
			rules: [
				{ token: "keyword", foreground: "569CD6" },
				{ token: "type.identifier", foreground: "4EC9B0" },
				{ token: "identifier", foreground: "9CDCFE" },
				{ token: "string", foreground: "CE9178" },
				{ token: "number", foreground: "B5CEA8" },
				{ token: "comment", foreground: "6A9955", fontStyle: "italic" },
				{ token: "operator", foreground: "D4D4D4" },
			],
			colors: {
				"editor.background": "#1e1e1e",
				"editor.foreground": "#d4d4d4",
				"editor.lineHighlightBackground": "#2a2a2a",
				"editorCursor.foreground": "#ffffff",
				"editorWhitespace.foreground": "#404040",
			},
		});

		monaco.editor.setTheme("terraform-dark");
	};

	const handleEditorChange = (value: string | undefined) => {
		if (value !== undefined) {
			onChange(value);
		}
	};

	return (
		<div className="h-full w-full border border-gray-700 rounded-lg overflow-hidden">
			<Editor
				height="100%"
				defaultLanguage="hcl"
				value={value}
				onChange={handleEditorChange}
				onMount={handleEditorDidMount}
				options={{
					minimap: { enabled: true },
					fontSize: 14,
					tabSize: 2,
					insertSpaces: true,
					wordWrap: "on",
					lineNumbers: "on",
					folding: true,
					automaticLayout: true,
					scrollBeyondLastLine: false,
					readOnly: readOnly,
					theme: "terraform-dark",
				}}
			/>
		</div>
	);
}

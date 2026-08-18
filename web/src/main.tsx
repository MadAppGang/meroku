import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";
import "./components/properties/property-panel.css";

const rootElement = document.getElementById("root");
if (!rootElement) {
	throw new Error("Root element #root not found — check index.html");
}

ReactDOM.createRoot(rootElement).render(
	<React.StrictMode>
		<App />
	</React.StrictMode>,
);

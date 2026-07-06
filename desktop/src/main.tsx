import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";

// Fonts are bundled (no network at runtime): Outfit for display, Inter for
// body, JetBrains Mono for numbers/labels — per design/tokens.json.
import "@fontsource/outfit/500.css";
import "@fontsource/outfit/600.css";
import "@fontsource/outfit/700.css";
import "@fontsource/inter/400.css";
import "@fontsource/inter/500.css";
import "@fontsource/inter/600.css";
import "@fontsource/inter/700.css";
import "@fontsource/jetbrains-mono/400.css";
import "@fontsource/jetbrains-mono/500.css";

import "./theme.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

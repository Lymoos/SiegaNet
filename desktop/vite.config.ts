import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Vite is only the UI bundler; Tauri wraps the built dist/ (see src-tauri/tauri.conf.json).
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  server: {
    port: 5173,
    strictPort: true,
  },
  build: {
    target: "es2021",
    outDir: "dist",
  },
});

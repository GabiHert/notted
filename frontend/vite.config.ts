/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
import { defineConfig } from "vite";

// `vite build --mode preview` swaps the two Wails modules for fixture mocks. Production
// builds (the default mode) never reference src/preview.
export default defineConfig(({ mode }) => ({
  plugins: [react()],
  resolve:
    mode === "preview"
      ? {
          alias: [
            { find: /^(\.\.\/)+wailsjs\/go\/wails\/App$/, replacement: resolve(__dirname, "src/preview/mockApp.ts") },
            { find: /^(\.\.\/)+wailsjs\/runtime\/runtime$/, replacement: resolve(__dirname, "src/preview/mockRuntime.ts") },
          ],
        }
      : {},
  test: { environment: "jsdom", globals: true, setupFiles: ["src/test/setup.ts"], css: true },
}));

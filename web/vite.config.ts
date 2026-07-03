import react from "@vitejs/plugin-react";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(dirname, "src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:23456",
      "/ws": {
        target: "ws://127.0.0.1:23456",
        ws: true,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return undefined;
          if (id.includes("/react/") || id.includes("/react-dom/") || id.includes("/scheduler/")) return "vendor-react";
          if (id.includes("/@reduxjs/") || id.includes("/react-redux/") || id.includes("/redux/")) return "vendor-state";
          if (
            id.includes("/marked/") ||
            id.includes("/marked-katex-extension/") ||
            id.includes("/katex/") ||
            id.includes("/highlight.js/")
          ) {
            return "vendor-markdown";
          }
          if (id.includes("/lucide-react/")) return "vendor-icons";
          return "vendor";
        },
      },
    },
  },
});

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
      "@openuidev/devtools": path.resolve(dirname, "src/lib/openui-devtools.ts"),
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
        // 显式隔离共享依赖，既避免普通页面预载 OpenUI，也避免组件库反向依赖消息分包。
        onlyExplicitManualChunks: true,
        manualChunks(id) {
          if (!id.includes("node_modules")) return undefined;
          if (id.includes("/@openuidev/")) return "vendor-openui";
          if (/\/node_modules\/(react|react-dom|scheduler)\//.test(id)) return "vendor-react";
          if (
            id.includes("/@reduxjs/") || id.includes("/react-redux/") || id.includes("/redux/") ||
            id.includes("/redux-thunk/") || id.includes("/immer/") || id.includes("/reselect/") ||
            id.includes("/use-sync-external-store/")
          ) return "vendor-state";
          if (id.includes("/clsx/") || id.includes("/tailwind-merge/") || id.includes("/class-variance-authority/")) return "vendor-utils";
          if (id.includes("/animejs/")) return "vendor-motion";
          if (
            id.includes("/marked/") ||
            id.includes("/marked-katex-extension/") ||
            id.includes("/dompurify/") ||
            id.includes("/katex/") ||
            id.includes("/highlight.js/")
          ) {
            return "vendor-markdown";
          }
          if (id.includes("/lucide-react/")) return "vendor-icons";
          if (id.includes("/zod/")) return "vendor-schema";
          return "vendor-ui";
        },
      },
    },
  },
});

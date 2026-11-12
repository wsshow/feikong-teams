import { expect, test } from "bun:test";
import config from "../../vite.config";

const output = config.build?.rollupOptions?.output;
if (!output || Array.isArray(output) || typeof output.manualChunks !== "function") throw new Error("manual chunk configuration is missing");
const manualChunks = output.manualChunks;
const context = { getModuleInfo: () => null, getModuleIds: function* () {} };
const chunkFor = (path: string) => manualChunks(`/project/web/node_modules/${path}/index.js`, context);

test("scoped React adapters do not enter the React runtime chunk", () => {
  // @floating-ui/react-dom 被误识别为 react-dom 时，会形成 UI 与 React 分包的循环初始化。
  expect(chunkFor("@floating-ui/react-dom")).toBe("vendor-ui");
  expect(chunkFor("@radix-ui/react-dialog")).toBe("vendor-ui");
  for (const name of ["react", "react-dom", "scheduler"]) expect(chunkFor(name)).toBe("vendor-react");
});

test("OpenUI dependencies stay outside application chunks and keep small shared utilities separate", () => {
  expect(chunkFor("@openuidev/react-ui")).toBe("vendor-openui");
  expect(chunkFor("goober")).toBe("vendor-ui");
  for (const name of ["clsx", "tailwind-merge", "class-variance-authority"]) expect(chunkFor(name)).toBe("vendor-utils");
  expect(manualChunks("/project/web/src/features/chat/openui/AssistantUIContent.tsx", context)).toBeUndefined();
});

import { assistantUILibrary, assistantUIPromptOptions } from "../src/features/chat/openui/library";

const target = new URL("../../internal/app/agent/catalog/workspaceassistant/openui_prompt.txt", import.meta.url);
const prompt = `${assistantUILibrary.prompt(assistantUIPromptOptions)}\n`;
if (process.argv.includes("--check")) {
  if (await Bun.file(target).text() !== prompt) throw new Error("assistant UI prompt is stale; run bun run generate:assistant-ui");
} else {
  await Bun.write(target, prompt);
}

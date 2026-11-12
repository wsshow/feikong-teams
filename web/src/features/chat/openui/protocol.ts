import { z } from "zod/v4";

export const maxUIProgramLength = 32000;
const actionPrefix = "【助手界面操作】\n";
const actionSchema = z.object({
  version: z.literal(1),
  source: z.string().min(1).max(1000),
  message: z.string().trim().min(1).max(1000),
  formState: z.record(z.string().max(80), z.record(z.string().max(80), z.object({ value: z.string().max(4000) }))).optional(),
}).strict();

export type UIAction = z.infer<typeof actionSchema>;

export function encodeUIAction(value: unknown) {
  const action = actionSchema.parse(value);
  const message = actionPrefix + JSON.stringify(action);
  if (message.length > 64000) throw new Error("表单内容过长，请精简后重试");
  return message;
}

export function decodeUIAction(message: string): UIAction | undefined {
  if (!message.startsWith(actionPrefix) || message.length > 64000) return undefined;
  try {
    const parsed = actionSchema.safeParse(JSON.parse(message.slice(actionPrefix.length)));
    return parsed.success ? parsed.data : undefined;
  } catch {
    return undefined;
  }
}

export function actionSummary(action: UIAction) {
  const values = Object.values(action.formState ?? {}).flatMap((fields) => Object.entries(fields).map(([name, field]) => `${name}：${field.value}`));
  return [action.message, ...values].join("\n");
}

export type UIMessagePart = { kind: "markdown"; content: string } | { kind: "openui"; content: string; complete: boolean; index: number };

// 逐行识别围栏，忽略普通代码块里的示例，也允许流式尚未闭合的 UI 块。
export function splitUIMessage(content: string): UIMessagePart[] {
  const result: UIMessagePart[] = [];
  let markdown = "";
  let program: string | undefined;
  let fence: { char: string; length: number; ui: boolean } | undefined;
  let index = 0;
  for (const line of content.match(/[^\n]*\n|[^\n]+$/g) ?? []) {
    const match = /^ {0,3}(`{3,}|~{3,})([^\r\n]*)\r?\n?$/.exec(line);
    if (!fence && match) {
      const ui = match[2].trim() === "openui-lang";
      fence = { char: match[1][0], length: match[1].length, ui };
      if (ui) {
        if (markdown) result.push({ kind: "markdown", content: markdown });
        markdown = "";
        program = "";
        continue;
      }
    } else if (fence && match && match[1][0] === fence.char && match[1].length >= fence.length && !match[2].trim()) {
      if (fence.ui) {
        result.push({ kind: "openui", content: program ?? "", complete: true, index: index++ });
        program = undefined;
        fence = undefined;
        continue;
      }
      fence = undefined;
    }
    if (program !== undefined) program += line;
    else markdown += line;
  }
  if (program !== undefined) result.push({ kind: "openui", content: program, complete: false, index });
  if (markdown) result.push({ kind: "markdown", content: markdown });
  return result;
}

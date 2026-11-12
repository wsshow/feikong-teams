import { z } from "zod/v4";

export const maxUIProgramLength = 32000;
const actionPrefix = "【助手界面操作】\n";
export type UIValue = string | number | boolean | null | UIValue[] | { [key: string]: UIValue };
const keySchema = z.string().max(80).refine((key) => !["__proto__", "constructor", "prototype"].includes(key));
function valueSchema(depth: number): z.ZodType<UIValue> {
  const primitive = z.union([z.string().max(4000), z.number(), z.boolean(), z.null(), z.undefined().transform(() => null), z.date().transform((date) => ({ $date: date.toISOString() }))]);
  if (!depth) return primitive;
  const child = valueSchema(depth - 1);
  return z.union([primitive, z.array(child).max(500), z.record(keySchema, child)]);
}
const uiValue = valueSchema(8);
const actionSchema = z.object({
  version: z.union([z.literal(1), z.literal(2)]),
  source: z.string().min(1).max(1000),
  message: z.string().trim().min(1).max(1000),
  formState: z.record(keySchema, z.record(keySchema, z.object({ value: uiValue }))).optional(),
  context: uiValue.optional(),
  state: z.record(keySchema, uiValue).optional(),
}).strict();

export type UIAction = z.infer<typeof actionSchema>;

export function encodeUIAction(value: unknown) {
  // 无 Form 的按钮会收到整个 store，包含 $绑定与顶层字段，单独保存这些状态。
  let normalized = value;
  if (value && typeof value === "object" && "version" in value && value.version === 2 && "formState" in value && value.formState && typeof value.formState === "object") {
    const forms: Record<string, unknown> = {};
    const state: Record<string, unknown> = {};
    for (const [name, item] of Object.entries(value.formState)) {
      keySchema.parse(name);
      if (name.startsWith("$") || !item || typeof item !== "object" || "value" in item) state[name] = item;
      else forms[name] = item;
    }
    normalized = { ...value, formState: forms, state };
  }
  const action = actionSchema.parse(normalized);
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
  const values = Object.values(action.formState ?? {}).flatMap((fields) => Object.entries(fields).map(([name, field]) => `${name}：${formatValue(field.value)}`));
  const fields = Object.entries(action.state ?? {}).flatMap(([name, field]) => !name.startsWith("$") && field && typeof field === "object" && !Array.isArray(field) && "value" in field ? [`${name}：${formatValue(field.value)}`] : []);
  return [action.message, ...values, ...fields].join("\n");
}

function formatValue(value: UIValue): string {
  if (value === null) return "未填写";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (Array.isArray(value)) return value.map(formatValue).join("、");
  if (typeof value === "object") {
    if (typeof value.$date === "string" && Object.keys(value).length === 1) return value.$date;
    return Object.entries(value).map(([key, item]) => `${key}：${formatValue(item)}`).join("；");
  }
  return String(value);
}

// DatePicker 使用 Date 对象，持久化为带标记的 JSON 后按原类型恢复。
export function restoreUIState(forms: UIAction["formState"], state?: UIAction["state"]) {
  function restore(value: UIValue): unknown {
    if (Array.isArray(value)) return value.map(restore);
    if (value && typeof value === "object") {
      if (typeof value.$date === "string" && Object.keys(value).length === 1 && !Number.isNaN(Date.parse(value.$date))) return new Date(value.$date);
      return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, restore(item)]));
    }
    return value;
  }
  if (!forms && !state) return undefined;
  return { ...Object.fromEntries(Object.entries(state ?? {}).map(([name, value]) => [name, restore(value)])), ...Object.fromEntries(Object.entries(forms ?? {}).map(([form, fields]) => [form, Object.fromEntries(Object.entries(fields).map(([name, field]) => [name, { value: restore(field.value) }]))])) };
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

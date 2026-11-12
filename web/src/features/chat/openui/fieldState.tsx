import { useEffect, type ReactNode } from "react";
import { useFormName, useGetFieldValue, useIsStreaming, useSetFieldValue, useStateField } from "@openuidev/react-lang";

// 上游部分控件只展示默认值，未修改时不会写入提交状态；在这里补齐。
export function FieldState({ type, props, children }: { type: string; props: Record<string, unknown>; children: ReactNode }) {
  const name = String(props.name ?? "");
  const form = useFormName();
  const getValue = useGetFieldValue();
  const setValue = useSetFieldValue();
  const streaming = useIsStreaming();
  const field = useStateField(name, props.value);
  const stored = getValue(form, name);
  let fallback = props.defaultValue;
  if (type === "SwitchGroup" || type === "CheckBoxGroup") {
    const items = (props.items ?? []) as { props?: { name?: string; defaultChecked?: boolean } }[];
    fallback = Object.fromEntries(items.filter((item) => item?.props?.name).map((item) => [item.props!.name!, item.props!.defaultChecked ?? false]));
  }
  if (type === "Slider" && fallback == null) fallback = [props.min ?? 0];
  const value = field.value ?? fallback;
  const encoded = JSON.stringify(value);
  const storedEncoded = JSON.stringify(stored);
  useEffect(() => {
    if (!streaming && value !== undefined && encoded !== storedEncoded && (field.isReactive || stored === undefined)) setValue(form, type, name, value, false);
  }, [streaming, encoded, storedEncoded, field.isReactive, stored, form, type, name, setValue, value]);
  return children;
}

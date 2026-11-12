import { createContext, useContext, type ReactNode, type SyntheticEvent } from "react";

export const InteractionDisabled = createContext(true);

// 同时覆盖原生控件、Radix 控件与通过 portal 弹出的选择器。
export function InteractionGuard({ children }: { children: ReactNode }) {
  const disabled = useContext(InteractionDisabled);
  function guard(event: SyntheticEvent) {
    if (disabled) { event.preventDefault(); event.stopPropagation(); }
  }
  return <fieldset disabled={disabled} aria-disabled={disabled} className="min-w-0 m-0 border-0 p-0"
    onClickCapture={guard} onPointerDownCapture={guard} onKeyDownCapture={guard} onChangeCapture={guard}>
    <div inert={disabled || undefined}>{children}</div>
  </fieldset>;
}

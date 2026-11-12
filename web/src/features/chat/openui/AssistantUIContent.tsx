import { Component, useCallback, useMemo, useRef, useState, type ReactNode } from "react";
import { Renderer, type ActionEvent, type OpenUIError, type ParseResult } from "@openuidev/react-lang";
import { ThemeProvider } from "@openuidev/react-ui";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { MarkdownContent } from "@/components/markdown/MarkdownContent";
import { assistantSessionID } from "@/lib/assistant";
import { sendChatMessage } from "../sendMessage";
import { assistantUILibrary } from "./library";
import { InteractionDisabled } from "./interaction";
import { assistantUITheme } from "./theme";
import { programProblem, safeExternalURL } from "./program";
import { decodeUIAction, encodeUIAction, restoreUIState, splitUIMessage } from "./protocol";
export { programProblem } from "./program";

export default function AssistantUIContent({ content, sourceID, streaming = false }: { content: string; sourceID: string; streaming?: boolean }) {
  const parts = useMemo(() => splitUIMessage(content), [content]);
  return <div className="space-y-3">{parts.map((part, index) => part.kind === "markdown"
    ? <MarkdownContent key={`text:${index}`} className="text-base leading-8 sm:text-lg sm:leading-9" content={part.content} />
    : <UIErrorBoundary key={`${sourceID}:${part.index}`}>
      <InteractiveBlock program={part.content} source={`${sourceID}:${part.index}`} streaming={streaming} complete={part.complete} />
    </UIErrorBoundary>)}
  </div>;
}

function InteractiveBlock({ program, source, streaming, complete }: { program: string; source: string; streaming: boolean; complete: boolean }) {
  const dispatch = useAppDispatch();
  const activeSession = useAppSelector((state) => state.chat.activeSessionID);
  const starting = useAppSelector((state) => state.chat.runningTasks[assistantSessionID]?.phase === "starting");
  const submitted = useAppSelector((state) => {
    if (state.chat.viewSessionID !== assistantSessionID) return undefined;
    const contents = [...state.chat.messages.filter((message) => message.role === "user").map((message) => message.content), ...state.chat.queue.map((item) => item.text || item.message || item.content || item.display_text)];
    return contents.map((content) => decodeUIAction(content ?? "")).find((action) => action?.source === source);
  }, (a, b) => JSON.stringify(a) === JSON.stringify(b));
  const restored = submitted;
  const [pending, setPending] = useState(false);
  const [sendError, setSendError] = useState("");
  const [renderErrors, setRenderErrors] = useState<OpenUIError[]>([]);
  const [hasRoot, setHasRoot] = useState(false);
  const [parseComplete, setParseComplete] = useState(false);
  const sending = useRef(false);
  const initialState = useRef(restoreUIState(restored?.formState, restored?.state));
  const latestState = useRef<Record<string, unknown>>(initialState.current ?? {});
  const onStateUpdate = useCallback((state: Record<string, unknown>) => { latestState.current = state; }, []);
  const onError = useCallback((errors: OpenUIError[]) => setRenderErrors((previous) => JSON.stringify(previous) === JSON.stringify(errors) ? previous : errors), []);
  const onParseResult = useCallback((result: ParseResult | null) => {
    setHasRoot(Boolean(result?.root));
    setParseComplete(Boolean(result?.root && !result.meta.incomplete && !result.meta.unresolved.length));
  }, []);
  const invalid = !streaming && (!complete || !parseComplete || renderErrors.some((error) => error.severity !== "warning"));
  const blocked = programProblem(program);
  const disabled = streaming || !complete || pending || Boolean(restored) || Boolean(starting) || activeSession !== assistantSessionID || invalid || !hasRoot;

  async function onAction(event: ActionEvent) {
    if (disabled || sending.current) return;
    if (event.type === "open_url") {
      const url = safeExternalURL(event.params?.url);
      if (url) window.open(url, "_blank", "noopener,noreferrer");
      else setSendError("链接地址无效，仅支持 HTTP/HTTPS 链接");
      return;
    }
    if (event.type !== "continue_conversation") return;
    sending.current = true;
    setPending(true);
    setSendError("");
    try {
      const message = encodeUIAction({ version: 2, source, message: event.humanFriendlyMessage, formState: { ...latestState.current, ...event.formState }, context: event.params?.context });
      await dispatch(sendChatMessage({ sessionID: assistantSessionID, message, mode: "team", agent: "" }));
    } catch (error) {
      setSendError(error instanceof Error ? error.message : String(error));
    } finally {
      sending.current = false;
      setPending(false);
    }
  }

  if (blocked || (!streaming && !complete)) return <UIFallback />;
  return <div className="min-w-0" aria-busy={streaming || pending}>
    <InteractionDisabled.Provider value={disabled}>
      <ThemeProvider cssSelector=".assistant-openui" lightTheme={assistantUITheme}>
        <div className="assistant-openui" hidden={invalid}>
          <Renderer library={assistantUILibrary} response={program} isStreaming={streaming} initialState={initialState.current} onStateUpdate={onStateUpdate} onAction={(event) => void onAction(event)} onError={onError} onParseResult={onParseResult} />
        </div>
      </ThemeProvider>
    </InteractionDisabled.Provider>
    {invalid || (!streaming && !hasRoot) ? <UIFallback /> : null}
    {streaming ? <p role="status" className="mt-2 text-xs text-muted-foreground">正在生成交互内容…</p> : null}
    {pending ? <p role="status" className="mt-2 text-xs text-muted-foreground">正在提交…</p> : restored ? <p role="status" className="mt-2 text-xs text-muted-foreground">已提交给助手</p> : null}
    {sendError ? <p role="alert" className="mt-2 text-sm text-destructive">{sendError}。填写内容已保留，可以重新提交。</p> : null}
  </div>;
}

function UIFallback() {
  return <div role="status" className="rounded-lg border border-border bg-muted/40 px-4 py-3 text-sm leading-6 text-muted-foreground">交互内容暂时无法显示。你可以直接在输入框回复，或请助手重新生成。</div>;
}

class UIErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() { return this.state.failed ? <UIFallback /> : this.props.children; }
}

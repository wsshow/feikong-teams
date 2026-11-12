// Redux 的异步错误会被序列化为普通对象，展示时只读取明确的文本消息。
export function errorMessage(error: unknown, fallback = "操作失败，请稍后重试") {
  const message = typeof error === "string"
    ? error
    : error && typeof error === "object" && "message" in error
      ? error.message
      : undefined;
  return typeof message === "string" && message.trim() && message !== "[object Object]"
    ? message
    : fallback;
}

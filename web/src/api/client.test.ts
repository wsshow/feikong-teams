import { expect, spyOn, test } from "bun:test";
import { APIError, get } from "./client";

test("malformed API error messages use readable fallbacks", async () => {
  for (const [status, message] of [[500, "服务器内部错误"], [200, "请求失败，请稍后重试"]] as const) {
    const fetch = spyOn(globalThis, "fetch").mockResolvedValue(Response.json({ code: 1, message: { detail: "failure" } }, { status }));
    try {
      await expect(get("/failure")).rejects.toThrow(message);
    } finally {
      fetch.mockRestore();
    }
  }
});

test("API errors retain the server's friendly message", async () => {
  const message = "这个会话的记录格式较旧，请新建会话继续使用。";
  const fetch = spyOn(globalThis, "fetch").mockImplementation(Object.assign(async () => Response.json({ code: 1, message }, { status: 409 }), { preconnect: globalThis.fetch.preconnect }));
  try {
    await expect(get("/legacy")).rejects.toBeInstanceOf(APIError);
    await expect(get("/legacy")).rejects.toThrow(message);
  } finally {
    fetch.mockRestore();
  }
});

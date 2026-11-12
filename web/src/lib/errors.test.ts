import { expect, test } from "bun:test";
import { errorMessage } from "./errors";

test("reads native and serialized error messages without coercing objects", () => {
  for (const error of [new Error("无法打开会话"), { name: "APIError", message: "无法打开会话" }, "无法打开会话"]) {
    expect(errorMessage(error)).toBe("无法打开会话");
  }
  for (const error of [undefined, null, {}, { message: {} }, { message: "" }, "[object Object]"]) {
    expect(errorMessage(error, "暂时无法打开，请稍后重试")).toBe("暂时无法打开，请稍后重试");
  }
});

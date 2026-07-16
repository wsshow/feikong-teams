import { expireAuthentication } from "@/lib/auth-session";
import { authToken } from "@/lib/storage";
import type { APIResponse } from "@/types/api";

export interface RequestOptions extends RequestInit {
  authFailure?: "expire" | "ignore";
}

export class APIError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "APIError";
    this.status = status;
  }
}

export async function request<T>(path: string, init: RequestOptions = {}): Promise<T> {
  const { authFailure = "expire", ...fetchInit } = init;
  const headers = new Headers(fetchInit.headers);
  const token = authToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (fetchInit.body && !headers.has("Content-Type") && !(fetchInit.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(path, { ...fetchInit, headers });
  if (!response.ok) {
    const message = await responseErrorMessage(response);
    if (response.status === 401 && authFailure === "expire") expireAuthentication();
    throw new APIError(message, response.status);
  }

  const payload = (await response.json()) as APIResponse<T>;
  if (payload.code !== 0) {
    throw new APIError(payload.message || "request failed", response.status);
  }
  return payload.data;
}

async function responseErrorMessage(response: Response) {
  const fallback = httpStatusMessage(response.status, response.statusText);
  const contentType = response.headers.get("Content-Type") || "";
  if (!contentType.includes("application/json")) {
    const text = await response.text().catch(() => "");
    return text.trim() || fallback;
  }
  try {
    const payload = (await response.json()) as Partial<APIResponse<unknown>>;
    return payload.message || fallback;
  } catch {
    return fallback;
  }
}

function httpStatusMessage(status: number, statusText: string) {
  switch (status) {
    case 400:
      return "请求参数错误";
    case 401:
      return "未登录或登录已过期";
    case 403:
      return "没有权限执行此操作";
    case 404:
      return "资源不存在";
    case 409:
      return "请求状态冲突";
    case 413:
      return "请求内容过大";
    case 500:
      return "服务器内部错误";
    case 502:
    case 503:
    case 504:
      return "服务暂时不可用";
    default:
      return statusText || "请求失败";
  }
}

export function get<T>(path: string, init: RequestOptions = {}) {
  return request<T>(path, init);
}

export function post<T>(path: string, body?: unknown, init: RequestOptions = {}) {
  return request<T>(path, {
    ...init,
    method: "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function put<T>(path: string, body?: unknown) {
  return request<T>(path, {
    method: "PUT",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function patch<T>(path: string, body?: unknown) {
  return request<T>(path, {
    method: "PATCH",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function del<T>(path: string) {
  return request<T>(path, { method: "DELETE" });
}

export function isAbortError(error: unknown) {
  return error instanceof Error && error.name === "AbortError";
}

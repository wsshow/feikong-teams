import { post } from "./client";

export interface LoginResponse {
  token: string;
}

export function login(username: string, password: string) {
  return post<LoginResponse>("/api/fkteams/login", { username, password }, { authFailure: "ignore" });
}

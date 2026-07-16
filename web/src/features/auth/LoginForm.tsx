import { useState, type FormEvent } from "react";
import { LogIn } from "lucide-react";
import { login } from "@/api/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function LoginForm({
  onAuthenticated,
  submitLabel = "登录",
}: {
  onAuthenticated: (token: string) => void;
  submitLabel?: string;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting) return;
    setError("");
    setSubmitting(true);
    try {
      const result = await login(username, password);
      if (!result.token) throw new Error("登录响应中缺少 Token");
      onAuthenticated(result.token);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="space-y-4" onSubmit={submit}>
      <Input
        className="h-12 px-4 text-base"
        value={username}
        onChange={(event) => setUsername(event.target.value)}
        placeholder="用户名"
        autoComplete="username"
        autoFocus
        disabled={submitting}
        required
      />
      <Input
        className="h-12 px-4 text-base"
        value={password}
        onChange={(event) => setPassword(event.target.value)}
        placeholder="密码"
        type="password"
        autoComplete="current-password"
        disabled={submitting}
        required
      />
      {error ? <div className="text-sm text-destructive">{error}</div> : null}
      <Button className="h-12 w-full text-base" type="submit" disabled={submitting}>
        <LogIn className="h-5 w-5" />
        {submitting ? "登录中" : submitLabel}
      </Button>
    </form>
  );
}

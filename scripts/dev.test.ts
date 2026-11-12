import { expect, test } from "bun:test";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

const launcher = path.join(import.meta.dir, "dev.sh");

async function eventually(check: () => Promise<boolean>) {
  for (let attempt = 0; attempt < 150; attempt++) {
    if (await check()) return;
    await Bun.sleep(20);
  }
  throw new Error("Timed out waiting for development processes");
}

function alive(pid: number) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

async function withServices(run: (fixture: {
  process: ReturnType<typeof Bun.spawn>;
  pids: number[];
}) => Promise<void>, fail = "") {
  // 带空格的路径同时验证命令路径被正确引用。
  const dir = await mkdtemp(path.join(tmpdir(), "fkteams dev-"));
  const service = path.join(dir, "fake service");
  await writeFile(service, `#!/usr/bin/env bash
set -eu
name=backend
if [ "$1" = run ]; then name=web; fi
sleep 300 &
child=$!
trap 'kill "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; exit 0' TERM INT HUP
printf '%s\\n%s\\n' "$$" "$child" > "$DEV_TEST_DIR/$name.pids"
if [ "$DEV_TEST_FAIL" = "$name" ]; then
    sleep 0.2
    kill "$child"
    wait "$child" 2>/dev/null || true
    exit 23
fi
wait "$child"
`, { mode: 0o755 });
  const child = Bun.spawn(["bash", launcher], {
    env: { ...process.env, BUN: service, AIR: service, DEV_TEST_DIR: dir, DEV_TEST_FAIL: fail },
    stdout: "ignore",
    stderr: "ignore",
  });
  let pids: number[] = [];
  try {
    await eventually(async () => {
      try {
        const files = await Promise.all(["web", "backend"].map(name => readFile(path.join(dir, `${name}.pids`), "utf8")));
        pids = files.flatMap(file => file.trim().split("\n").map(Number));
        return pids.length === 4 && pids.every(pid => pid > 0);
      } catch {
        return false;
      }
    });
    await run({ process: child, pids });
  } finally {
    child.kill("SIGTERM");
    await child.exited;
    for (const pid of pids) {
      if (alive(pid)) process.kill(pid, "SIGKILL");
    }
    await rm(dir, { recursive: true, force: true });
  }
}

for (const signal of ["SIGINT", "SIGTERM"] as const) {
  test(`${signal} stops both services and their children without stopping unrelated processes`, async () => {
    const unrelated = Bun.spawn(["sleep", "300"]);
    try {
      await withServices(async ({ process: child, pids }) => {
        child.kill(signal);
        expect(await child.exited).toBe(signal === "SIGINT" ? 130 : 143);
        await eventually(async () => pids.every(pid => !alive(pid)));
        expect(alive(unrelated.pid)).toBe(true);
      });
    } finally {
      unrelated.kill();
      await unrelated.exited;
    }
  }, 10000);
}

for (const service of ["web", "backend"]) {
  test(`${service} failure stops the other service and preserves the exit code`, async () => {
    await withServices(async ({ process: child, pids }) => {
      expect(await child.exited).toBe(23);
      await eventually(async () => pids.every(pid => !alive(pid)));
    }, service);
  }, 10000);
}

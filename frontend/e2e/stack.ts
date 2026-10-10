import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdirSync, openSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { ADMIN, API_PORT, API_URL, DATABASE_URL, PG_CONTAINER, PG_PORT } from "./env";

const here = path.dirname(fileURLToPath(import.meta.url));
const backendDir = path.resolve(here, "../../backend");
const tmpDir = path.resolve(here, ".tmp");

let api: ChildProcess | null = null;

function docker(args: string[], opts: { allowFail?: boolean } = {}): string {
  try {
    return execFileSync("docker", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
  } catch (error) {
    if (opts.allowFail) return "";
    throw new Error(`docker ${args.join(" ")} failed: ${(error as Error).message}`, { cause: error });
  }
}

async function waitFor(label: string, check: () => Promise<boolean> | boolean, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      if (await check()) return;
    } catch {
      // keep polling
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`Timed out waiting for ${label}`);
}

/** Starts a throwaway Postgres, builds and starts the Go API against it. */
export async function startStack(): Promise<void> {
  docker(["rm", "-f", PG_CONTAINER], { allowFail: true });
  docker([
    "run", "-d", "--name", PG_CONTAINER,
    "-e", "POSTGRES_USER=inventario", "-e", "POSTGRES_PASSWORD=inventario", "-e", "POSTGRES_DB=inventario",
    "-p", `127.0.0.1:${PG_PORT}:5432`,
    "postgres:16",
  ]);
  await waitFor(
    "postgres",
    () => {
      docker(["exec", PG_CONTAINER, "pg_isready", "-U", "inventario", "-d", "inventario"]);
      // pg_isready can answer before the init script restarts the server; confirm with a real query.
      return docker(["exec", PG_CONTAINER, "psql", "-U", "inventario", "-d", "inventario", "-tAc", "select 1"]).trim() === "1";
    },
    60_000,
  );

  mkdirSync(tmpDir, { recursive: true });
  const binary = path.join(tmpDir, "api");
  execFileSync("go", ["build", "-o", binary, "./cmd/api"], { cwd: backendDir, stdio: "inherit" });

  const log = openSync(path.join(tmpDir, "api.log"), "w");
  api = spawn(binary, [], {
    cwd: backendDir,
    stdio: ["ignore", log, log],
    env: {
      ...process.env,
      PORT: String(API_PORT),
      DATABASE_URL,
      JWT_SECRET: "e2e-only-jwt-secret-at-least-32-characters-long",
      COOKIE_SECURE: "false",
      BOOTSTRAP_ADMIN_EMAIL: ADMIN.email,
      BOOTSTRAP_ADMIN_PASSWORD: ADMIN.password,
    },
  });
  await waitFor("api", async () => (await fetch(`${API_URL}/api/v1/health`)).ok, 30_000);
}

export function stopStack(): void {
  if (api && !api.killed) api.kill("SIGTERM");
  api = null;
  docker(["rm", "-f", PG_CONTAINER], { allowFail: true });
}

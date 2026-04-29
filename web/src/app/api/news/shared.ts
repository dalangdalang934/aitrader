import { readFile } from "node:fs/promises";
import path from "node:path";

const CACHE_DIR_CANDIDATES = [
  path.resolve(process.cwd(), "..", "data", "news"),
  path.resolve(process.cwd(), "data", "news"),
];

export async function readNewsCache<T>(filename: string): Promise<T | null> {
  for (const dir of CACHE_DIR_CANDIDATES) {
    try {
      const file = path.join(dir, filename);
      const raw = await readFile(file, "utf8");
      return JSON.parse(raw) as T;
    } catch {
      // Try the next candidate.
    }
  }
  return null;
}

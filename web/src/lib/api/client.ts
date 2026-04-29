export const BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL || "/api/backend";

export async function fetcher<T = unknown>(
  url: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(url, {
    ...init,
    // Allow the browser HTTP cache to satisfy short‑interval polling.
    // Combined with Cache-Control from our proxy, this avoids hitting Vercel at all
    // when data is fresh, dramatically reducing Fast Data Transfer.
    cache: init?.cache ?? "default",
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`Request failed ${res.status}: ${text || res.statusText}`);
  }
  try {
    return await res.json();
  } catch {
    throw new Error(`Invalid JSON response from ${url}`);
  }
}

export const apiUrl = (path: string) => `${BASE_URL}${path}`;

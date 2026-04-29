import { NextResponse } from "next/server";
import { readNewsCache } from "./shared";

export const runtime = "nodejs";

const RAW_BASE = process.env.API_BASE_URL || "http://localhost:8080";
const BASE = RAW_BASE.endsWith("/") ? RAW_BASE.slice(0, -1) : RAW_BASE;
const API_BASE = BASE.endsWith("/api") ? BASE : `${BASE}/api`;

export async function GET() {
  try {
    const upstream = await fetch(`${API_BASE}/news/digests`, {
      cache: "no-store",
      headers: {
        Accept: "application/json",
      },
    });

    if (!upstream.ok) {
      const fallback = await readNewsCache<{
        digests?: unknown[];
        updated_at?: string;
        count?: number;
      }>("digests.json");
      if (fallback) {
        const digests = Array.isArray(fallback.digests) ? fallback.digests : [];
        return NextResponse.json(
          {
            digests,
            count: fallback.count ?? digests.length,
            updated_at: fallback.updated_at,
            stale: true,
          },
          {
            headers: {
              "cache-control": "no-store",
            },
          },
        );
      }

      return NextResponse.json(
        { digests: [], count: 0, updated_at: null, available: false },
        {
          headers: {
            "cache-control": "no-store",
          },
        },
      );
    }

    let payload;
    try {
      payload = await upstream.json();
    } catch {
      return NextResponse.json(
        { error: "invalid JSON from news upstream" },
        { status: 502 },
      );
    }
    return NextResponse.json(payload, {
      headers: {
        "cache-control": "no-store",
      },
    });
  } catch (error: unknown) {
    const fallback = await readNewsCache<{
      digests?: unknown[];
      updated_at?: string;
      count?: number;
    }>("digests.json");
    if (fallback) {
      const digests = Array.isArray(fallback.digests) ? fallback.digests : [];
      return NextResponse.json(
        {
          digests,
          count: fallback.count ?? digests.length,
          updated_at: fallback.updated_at,
          stale: true,
        },
        {
          headers: {
            "cache-control": "no-store",
          },
        },
      );
    }

    const message = error instanceof Error ? error.message : String(error);
    return NextResponse.json(
      {
        digests: [],
        count: 0,
        updated_at: null,
        available: false,
        detail: message,
      },
      {
        headers: {
          "cache-control": "no-store",
        },
      },
    );
  }
}

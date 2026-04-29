import { NextResponse } from "next/server";
import { readNewsCache } from "../shared";

export const runtime = "nodejs";

const RAW_BASE = process.env.API_BASE_URL || "http://localhost:8080";
const BASE = RAW_BASE.endsWith("/") ? RAW_BASE.slice(0, -1) : RAW_BASE;
const API_BASE = BASE.endsWith("/api") ? BASE : `${BASE}/api`;

export async function GET() {
  try {
    const upstream = await fetch(`${API_BASE}/news/outlook`, {
      cache: "no-store",
      headers: { Accept: "application/json" },
    });

    if (!upstream.ok) {
      const fallback = await readNewsCache<{
        generated_at?: string;
        valid_until?: string;
      }>("outlook.json");
      if (fallback) {
        return NextResponse.json(
          {
            outlook: fallback,
            available: true,
            stale: true,
            updated_at: fallback.generated_at,
          },
          {
            headers: { "cache-control": "no-store" },
          },
        );
      }

      return NextResponse.json(
        {
          outlook: null,
          available: false,
          updated_at: null,
        },
        {
          headers: { "cache-control": "no-store" },
        },
      );
    }

    let payload;
    try {
      payload = await upstream.json();
    } catch {
      return NextResponse.json(
        { error: "invalid JSON from outlook upstream" },
        { status: 502 },
      );
    }
    return NextResponse.json(payload, {
      headers: { "cache-control": "no-store" },
    });
  } catch (error: unknown) {
    const fallback = await readNewsCache<{
      generated_at?: string;
      valid_until?: string;
    }>("outlook.json");
    if (fallback) {
      return NextResponse.json(
        {
          outlook: fallback,
          available: true,
          stale: true,
          updated_at: fallback.generated_at,
        },
        {
          headers: { "cache-control": "no-store" },
        },
      );
    }

    const message = error instanceof Error ? error.message : String(error);
    return NextResponse.json(
      {
        outlook: null,
        available: false,
        updated_at: null,
        detail: message,
      },
      {
        headers: { "cache-control": "no-store" },
      },
    );
  }
}

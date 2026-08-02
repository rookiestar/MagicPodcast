import { NextResponse } from "next/server";

const MOCK_COVER_SOURCES: Record<string, string> = {
  "1": "https://images.unsplash.com/photo-1478737270239-2f02b77fc618?auto=format&fit=crop&w=800&q=85",
  "2": "https://images.unsplash.com/photo-1590602847861-f357a9332bbc?auto=format&fit=crop&w=800&q=85",
  "3": "https://images.unsplash.com/photo-1589903308904-1010c2294adc?auto=format&fit=crop&w=800&q=85",
  "5": "https://images.unsplash.com/photo-1495446815901-a7297e633e8d?auto=format&fit=crop&w=800&q=85",
};

function isMockApiEnabled() {
  return ["1", "true", "yes"].includes(
    String(process.env.MAGICPODCAST_FRONTEND_MOCK_API || "").toLowerCase(),
  );
}

export async function GET(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  if (!isMockApiEnabled()) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const { id } = await context.params;
  const source = MOCK_COVER_SOURCES[id];
  if (!source) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const response = await fetch(source, {
    headers: { Accept: "image/avif,image/webp,image/jpeg" },
    cache: "force-cache",
  });
  if (!response.ok || !response.body) {
    return NextResponse.json({ error: "Cover unavailable" }, { status: 502 });
  }

  return new Response(response.body, {
    headers: {
      "Cache-Control": "public, max-age=86400, stale-while-revalidate=604800",
      "Content-Type": response.headers.get("content-type") || "image/jpeg",
    },
  });
}

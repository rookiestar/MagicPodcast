import { NextResponse } from "next/server";
import { handleMockRequest } from "@/lib/mockApi";

type RouteContext = {
  params: Promise<{ path?: string[] }>;
};

async function handle(request: Request, context: RouteContext) {
  const { path = [] } = await context.params;
  const url = new URL(request.url);
  let body: unknown;

  if (!["GET", "HEAD"].includes(request.method)) {
    try {
      body = await request.json();
    } catch {
      body = undefined;
    }
  }

  const response = await handleMockRequest({
    method: request.method,
    pathname: `/api/v1/${path.join("/")}`,
    search: url.search,
    body,
  });

  return NextResponse.json(response.body, { status: response.status });
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;

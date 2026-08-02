import { NextResponse } from "next/server";

export function GET() {
  return NextResponse.json({
    status: "ok",
    mode: "mock",
    database: "not connected",
  });
}

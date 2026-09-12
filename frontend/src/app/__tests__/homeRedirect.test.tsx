import { expect, it, vi } from "vitest";
import Home from "../page";
const redirect = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({ redirect }));
it("keeps discovery parameters including repeated workflows at the root alias", async () => {
  await Home({ searchParams: Promise.resolve({ filter: "unread", workflow: ["1", "2"], report: "3" }) });
  expect(redirect).toHaveBeenCalledWith("/discovery?filter=unread&workflow=1&workflow=2&report=3");
});

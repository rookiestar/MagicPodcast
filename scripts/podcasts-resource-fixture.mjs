#!/usr/bin/env node

import http from "node:http";
import { readFile } from "node:fs/promises";

const host = "127.0.0.1";
const port = Number.parseInt(process.env.PORT || "18080", 10);
const cover = await readFile(
  new URL(
    "../frontend/public/brand/magicpodcast-tuning-mark.png",
    import.meta.url,
  ),
);

const podcasts = Array.from({ length: 15 }, (_, index) => ({
  id: index + 1,
  title: `动态节目标题 ${index + 1}：系统字体资源预算验证`,
  author: `作者 ${index + 1}`,
  description:
    "这是用于本地浏览器资源预算与响应式检查的播客简介，不依赖真实数据库。",
  cover_url: `https://i.typlog.com/fixture/v2/cover-${index + 1}.png`,
  newest_episode_date: new Date().toISOString(),
  episode_count: 20 + index,
  tags: [],
}));

const summary = {
  success: true,
  data: {
    counts: {
      inbox: 2,
      focus: 1,
      someday: 0,
      done: 3,
    },
  },
};

function sendJson(response, payload) {
  response.writeHead(200, {
    "content-type": "application/json; charset=utf-8",
    "cache-control": "no-store",
  });
  response.end(JSON.stringify(payload));
}

const server = http.createServer((request, response) => {
  const url = new URL(request.url || "/", `http://${host}:${port}`);

  if (url.pathname === "/health") {
    sendJson(response, { status: "ok" });
    return;
  }

  if (url.pathname === "/api/v1/podcasts") {
    const pageSize = Number.parseInt(
      url.searchParams.get("page_size") || "15",
      10,
    );
    sendJson(response, {
      success: true,
      data: podcasts.slice(0, pageSize),
      pagination: {
        page: 1,
        page_size: pageSize,
        total: podcasts.length,
        total_pages: 1,
      },
    });
    return;
  }

  if (url.pathname === "/api/v1/tags") {
    sendJson(response, { success: true, data: [] });
    return;
  }

  if (url.pathname === "/api/v1/consumption/summary") {
    sendJson(response, summary);
    return;
  }

  if (url.pathname === "/images/proxy") {
    response.writeHead(200, {
      "content-type": "image/png",
      "cache-control": "public, max-age=31536000, immutable",
    });
    response.end(cover);
    return;
  }

  response.writeHead(404, { "content-type": "text/plain; charset=utf-8" });
  response.end("Not Found");
});

server.listen(port, host, () => {
  console.log(`Podcast resource fixture listening on http://${host}:${port}`);
});

function shutdown() {
  server.close(() => process.exit(0));
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

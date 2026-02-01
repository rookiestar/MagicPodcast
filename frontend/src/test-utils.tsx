import React, { ReactElement } from "react";
import { render, RenderOptions } from "@testing-library/react";
import { ConfigProvider } from "antd";

// 自定义渲染函数，可以包含全局 Provider
interface CustomRenderOptions extends Omit<RenderOptions, "wrapper"> {
  wrapper?: React.ComponentType<{ children: React.ReactNode }>;
}

export function renderWithProviders(
  ui: ReactElement,
  options?: CustomRenderOptions,
) {
  const AllTheProviders = ({ children }: { children: React.ReactNode }) => {
    // 在这里添加你的 Provider
    return <>{children}</>;
  };

  return render(ui, { wrapper: AllTheProviders, ...options });
}

// 重新导出所有测试工具
export * from "@testing-library/react";
export { renderWithProviders as render };

// Mock data generators
export const createMockTag = (overrides = {}) => ({
  id: 1,
  name: "Test Tag",
  color: "#ff0000",
  description: "Test description",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  ...overrides,
});

export const createMockPodcast = (overrides = {}) => ({
  id: 1,
  xyz_id: "test-xyz-id",
  title: "Test Podcast",
  feed_url: "https://example.com/feed.xml",
  cover_url: "https://example.com/cover.jpg",
  description: "Test description",
  author: "Test Author",
  episode_count: 100,
  is_subscribed: true,
  is_dead: false,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  ...overrides,
});

export const createMockWorkflow = (overrides = {}) => ({
  id: 1,
  name: "Test Workflow",
  description: "Test description",
  schedule: "0 0 2 * * *",
  scope_type: "all_subscribed",
  is_enabled: true,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  scope_config: {},
  rules_config: {},
  ...overrides,
});

export const createMockEpisode = (overrides = {}) => ({
  id: 1,
  xyz_id: "episode-xyz-id",
  podcast_id: 1,
  title: "Test Episode",
  medium_url: "https://example.com/episode.mp3",
  show_notes: "Test show notes",
  published_date: "2024-01-01T00:00:00Z",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  ...overrides,
});

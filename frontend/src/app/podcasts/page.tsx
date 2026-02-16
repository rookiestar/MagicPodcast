import dynamic from "next/dynamic";

// 禁用 SSR 以避免 hydration 错误
// 原因：useBreakpoint hook 使用 window.innerWidth，在服务端不可用
const PodcastsContent = dynamic(
  () => import("./PodcastsContent"),
  { ssr: false }
);

export default function PodcastsPage() {
  return <PodcastsContent />;
}

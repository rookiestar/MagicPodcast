import PodcastsContent from "./PodcastsContent";

// PodcastsContent is a client component and owns its browser-only breakpoint hook.
// Importing it directly keeps this route compatible with Next.js 16 Server Components.

export default function PodcastsPage() {
  return <PodcastsContent />;
}

import { Suspense } from "react";
import CollectionsContent from "./CollectionsContent";

export default function CollectionsPage() {
  return (
    <Suspense fallback={<p role="status">正在读取清单…</p>}>
      <CollectionsContent />
    </Suspense>
  );
}

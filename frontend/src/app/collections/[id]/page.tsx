"use client";

import { useParams } from "next/navigation";
import CollectionDetailContent, {
  parseCollectionID,
} from "./CollectionDetailContent";

export default function CollectionDetailPage() {
  const params = useParams();
  const collectionID = parseCollectionID(params?.id);
  return <CollectionDetailContent collectionID={collectionID} />;
}

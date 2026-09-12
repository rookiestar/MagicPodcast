"use client";

import SearchSidebar from "@/components/SearchSidebar";
import { useRouter } from "next/navigation";

export default function SearchPage() {
  const router = useRouter();
  return <SearchSidebar isOpen onClose={() => router.replace("/discovery")} standalone />;
}

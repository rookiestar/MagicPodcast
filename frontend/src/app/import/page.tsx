import ImportPageClient from "./ImportPageClient";

export default async function ImportPage({ searchParams }: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const params = await searchParams;
  return <ImportPageClient initialTab={params.tab === "sync" ? "sync" : "import"} />;
}

const MAX_SEARCH_HISTORY = 6;
const STORAGE_KEY = "podcast_search_history";

export function getSearchHistory(): string[] {
  if (typeof window === "undefined") return [];

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return [];

    const parsed = JSON.parse(stored);
    if (!Array.isArray(parsed)) return [];

    return parsed.filter((item): item is string => typeof item === "string");
  } catch {
    return [];
  }
}

function saveSearchHistory(history: string[]) {
  if (typeof window === "undefined") return;

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(history));
  } catch (error) {
    console.error("Failed to save search history:", error);
  }
}

export function addToSearchHistory(query: string): string[] {
  const normalizedQuery = query.trim();
  if (!normalizedQuery) return getSearchHistory();

  const history = getSearchHistory();
  const filtered = history.filter((item) => item !== normalizedQuery);
  const newHistory = [normalizedQuery, ...filtered].slice(
    0,
    MAX_SEARCH_HISTORY,
  );

  saveSearchHistory(newHistory);
  return newHistory;
}

export function clearSearchHistory() {
  saveSearchHistory([]);
}

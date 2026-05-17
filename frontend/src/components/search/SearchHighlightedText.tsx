import { getSearchTextHighlightParts } from "@/lib/searchResultDisplay";

interface SearchHighlightedTextProps {
  text: string;
  keyword: string;
}

export function SearchHighlightedText({
  text,
  keyword,
}: SearchHighlightedTextProps) {
  return (
    <>
      {getSearchTextHighlightParts(text, keyword).map((part, index) => {
        if (!part.highlighted) return part.text;

        return (
          <mark
            key={`${part.text}-${index}`}
            className="bg-yellow-200 dark:bg-yellow-800 rounded px-0.5"
          >
            {part.text}
          </mark>
        );
      })}
    </>
  );
}

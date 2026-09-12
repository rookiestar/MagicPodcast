"use client";

import { createContext, useContext, useRef, ReactNode } from "react";

import { usePathname } from "next/navigation";
import { closeTo, navigate } from "@/lib/navigation";

interface SearchContextType {
  isSearchOpen: boolean;
  openSearch: () => void;
  closeSearch: () => void;
}

const SearchContext = createContext<SearchContextType | undefined>(undefined);

export function SearchProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const isSearchOpen = pathname === "/search";
  const returnHref = useRef("/discovery");
  const openSearch = () => {
    if (isSearchOpen) return;
    returnHref.current = window.location.pathname + window.location.search + window.location.hash;
    navigate("/search");
  };
  const closeSearch = () => closeTo(returnHref.current);

  return (
    <SearchContext.Provider value={{ isSearchOpen, openSearch, closeSearch }}>
      {children}
    </SearchContext.Provider>
  );
}

export function useSearch() {
  const context = useContext(SearchContext);
  if (context === undefined) {
    throw new Error("useSearch must be used within a SearchProvider");
  }
  return context;
}

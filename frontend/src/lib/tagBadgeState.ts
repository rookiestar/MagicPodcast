import type { CSSProperties } from "react";
import type { Tag } from "@/types";

export type TagBadgeSize = "sm" | "md" | "lg";
export type TagBadgeVariant = "colorful" | "simple";

const TAG_BADGE_SIZE_CLASSES: Record<TagBadgeSize, string> = {
  sm: "text-xs px-2 py-0.5",
  md: "text-sm px-3 py-1",
  lg: "text-base px-4 py-1.5",
};

const SIMPLE_REMOVE_BUTTON_CLASS =
  "ml-1 hover:bg-slate-300 rounded-full p-2 transition-colors focus:outline-none active:scale-95";

const COLORFUL_REMOVE_BUTTON_CLASS = `
  hover:bg-white/50 rounded-full p-2
  transition-colors duration-150
  focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-current
  active:scale-95
`;

export function getTagBadgeSizeClass(size: TagBadgeSize) {
  return TAG_BADGE_SIZE_CLASSES[size];
}

export function getTagRemoveTitle(tagName: string) {
  return `移除 "${tagName}" 标签`;
}

export function shouldStopTagRemovePropagation(variant: TagBadgeVariant) {
  return variant === "simple";
}

export function getTagRemoveButtonClass(variant: TagBadgeVariant) {
  return variant === "simple"
    ? SIMPLE_REMOVE_BUTTON_CLASS
    : COLORFUL_REMOVE_BUTTON_CLASS;
}

export function getTagRemoveButtonStyle(
  tagColor: string,
  variant: TagBadgeVariant,
): CSSProperties {
  return {
    ...(variant === "simple" ? {} : { color: tagColor }),
    minWidth: "44px",
    minHeight: "44px",
  };
}

interface TagBadgeComparableProps {
  tag: Pick<Tag, "id" | "name" | "color">;
  size?: TagBadgeSize;
  removable?: boolean;
  variant?: TagBadgeVariant;
}

export function areTagBadgePropsEqual(
  prevProps: Readonly<TagBadgeComparableProps>,
  nextProps: Readonly<TagBadgeComparableProps>,
) {
  return (
    prevProps.tag.id === nextProps.tag.id &&
    prevProps.tag.name === nextProps.tag.name &&
    prevProps.tag.color === nextProps.tag.color &&
    prevProps.size === nextProps.size &&
    prevProps.removable === nextProps.removable &&
    prevProps.variant === nextProps.variant
  );
}

"use client";

import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";

const menuItemsSelector =
  '[role="menuitem"]:not([disabled]), [role="menuitemradio"]:not([disabled])';

// Shared behavior for compact paper menus: a trigger button opens a list of
// menu items that supports roving arrow-key focus, Home/End, Enter/Space
// activation, dismissal on Escape/outside press/focus departure, and trigger
// focus restoration for explicit closes. Escape is handled in the document
// capture phase so an open menu never also closes the surrounding detail
// dialog.
export function useMenuPopover() {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuId = useId();

  const closeMenu = useCallback(() => {
    setOpen(false);
    triggerRef.current?.focus({ preventScroll: true });
  }, []);

  const toggleMenu = useCallback(() => {
    setOpen((current) => !current);
  }, []);

  useEffect(() => {
    if (!open) return;
    const menu = menuRef.current;
    (
      menu?.querySelector<HTMLButtonElement>('[aria-checked="true"]') ??
      menu?.querySelector<HTMLButtonElement>(menuItemsSelector)
    )?.focus({ preventScroll: true });

    const closeOnOutsidePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (
        target instanceof Node &&
        (menuRef.current?.contains(target) ||
          triggerRef.current?.contains(target))
      ) {
        return;
      }
      setOpen(false);
    };
    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      closeMenu();
    };
    const closeOnFocusOutside = (event: FocusEvent) => {
      const target = event.target;
      if (
        target instanceof Node &&
        (menuRef.current?.contains(target) ||
          triggerRef.current?.contains(target))
      ) {
        return;
      }
      setOpen(false);
    };
    document.addEventListener("pointerdown", closeOnOutsidePointerDown, true);
    document.addEventListener("keydown", closeOnEscape, true);
    document.addEventListener("focusin", closeOnFocusOutside, true);
    return () => {
      document.removeEventListener(
        "pointerdown",
        closeOnOutsidePointerDown,
        true,
      );
      document.removeEventListener("keydown", closeOnEscape, true);
      document.removeEventListener("focusin", closeOnFocusOutside, true);
    };
  }, [closeMenu, open]);

  const handleMenuKeyDown = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      const items = Array.from(
        menuRef.current?.querySelectorAll<HTMLButtonElement>(
          menuItemsSelector,
        ) ?? [],
      );
      if (items.length === 0) return;
      const currentIndex = items.indexOf(
        document.activeElement as HTMLButtonElement,
      );
      let nextIndex: number;
      switch (event.key) {
        case "ArrowDown":
          nextIndex = (currentIndex + 1) % items.length;
          break;
        case "ArrowUp":
          nextIndex = (currentIndex - 1 + items.length) % items.length;
          break;
        case "Home":
          nextIndex = 0;
          break;
        case "End":
          nextIndex = items.length - 1;
          break;
        default:
          return;
      }
      event.preventDefault();
      items[nextIndex]?.focus();
    },
    [],
  );

  return {
    open,
    menuId,
    triggerRef,
    menuRef,
    closeMenu,
    toggleMenu,
    handleMenuKeyDown,
  };
}

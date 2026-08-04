"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const navItems = [
  { label: "首页", href: "/" },
  { label: "播客", href: "/podcasts" },
  { label: "标签", href: "/tags" },
  { label: "工作流", href: "/workflows" },
  { label: "导入", href: "/import" },
];

interface MobileBottomNavProps {
  onSearchClick?: () => void;
}

function isCurrentPath(pathname: string, href: string) {
  if (href === "/") {
    return pathname === "/" || pathname.startsWith("/discovery");
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

export default function MobileBottomNav({
  onSearchClick,
}: MobileBottomNavProps) {
  const pathname = usePathname();

  return (
    <nav className="mobile-bottom-nav" aria-label="移动导航">
      <div>
        {navItems.map((item) => {
          const isActive = isCurrentPath(pathname, item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              prefetch={false}
              aria-current={isActive ? "page" : undefined}
            >
              {item.label}
            </Link>
          );
        })}
        <button type="button" aria-label="搜索" onClick={onSearchClick}>
          搜索
        </button>
      </div>
    </nav>
  );
}

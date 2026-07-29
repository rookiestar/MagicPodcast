"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { CompactLogo } from "./Logo";

const navItems = [
  { label: "首页", href: "/" },
  { label: "播客", href: "/podcasts" },
  { label: "标签", href: "/tags" },
  { label: "工作流", href: "/workflows" },
  { label: "导入", href: "/import" },
];

interface AppNavbarProps {
  onSearchClick?: () => void;
  syncStatus?: {
    isSyncing: boolean;
    lastSync?: string;
  };
}

function isCurrentPath(pathname: string, href: string) {
  if (href === "/") {
    return pathname === "/" || pathname.startsWith("/discovery");
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

export default function AppNavbar({
  onSearchClick,
  syncStatus,
}: AppNavbarProps) {
  const pathname = usePathname();

  return (
    <nav className="app-navbar" aria-label="主导航">
      <div className="app-navbar-inner">
        <Link href="/" prefetch={false} className="app-navbar-brand">
          <CompactLogo />
        </Link>
        <div className="app-navbar-links">
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
        </div>
        <div className="app-navbar-actions">
          <button type="button" aria-label="搜索" onClick={onSearchClick}>
            搜索
          </button>
          {syncStatus && (
            <span className="app-navbar-sync" role="status">
              {syncStatus.isSyncing
                ? "同步中"
                : syncStatus.lastSync
                  ? "已同步"
                  : "未同步"}
            </span>
          )}
        </div>
      </div>
    </nav>
  );
}

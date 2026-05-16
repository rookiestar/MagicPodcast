import type { Metadata, Viewport } from "next";
import "./globals.css";
import { ToastProvider } from "@/lib/toast";
import { SearchProvider } from "@/contexts/SearchContext";

export const metadata: Metadata = {
  title: "MagicPodcast - 个人播客管理",
  description: "个人播客管理与自动化处理工具",
  icons: {
    icon: "/favicon.svg",
    apple: "/favicon.svg",
  },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  maximumScale: 5,
  viewportFit: "cover",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body>
        <SearchProvider>
          <ToastProvider>{children}</ToastProvider>
        </SearchProvider>
      </body>
    </html>
  );
}

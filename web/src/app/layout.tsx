import Script from "next/script";
import type { Metadata } from "next";
import "./globals.css";
export const metadata: Metadata = {
  title: "safe-devops · 安全运维控制台",
  description: "独立审批、短期授权、服务器主动连接的开源运维控制平面",
};
export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN" data-theme="dark" suppressHydrationWarning>
      <body>
        <Script src="/preferences.js" strategy="beforeInteractive" />
        {children}
      </body>
    </html>
  );
}

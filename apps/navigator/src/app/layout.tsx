import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: {
    default: "Detent Navigator",
    template: "%s | Detent",
  },
  description:
    "Authenticate and manage your Detent CLI sessions. Securely connect your development environment to Detent services.",
  applicationName: "Detent Navigator",
  keywords: ["detent", "cli", "authentication", "developer tools"],
  authors: [{ name: "Detent" }],
  creator: "Detent",
  openGraph: {
    type: "website",
    siteName: "Detent Navigator",
    title: "Detent Navigator",
    description:
      "Authenticate and manage your Detent CLI sessions. Securely connect your development environment to Detent services.",
  },
  twitter: {
    card: "summary",
    title: "Detent Navigator",
    description:
      "Authenticate and manage your Detent CLI sessions. Securely connect your development environment to Detent services.",
  },
  robots: {
    index: true,
    follow: true,
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased`}
      >
        {children}
      </body>
    </html>
  );
}

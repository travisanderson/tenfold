import { WorkspaceProvider } from "@/components/workspace";
import type { Metadata } from "next";
import "@fontsource-variable/bricolage-grotesque";
import "@fontsource-variable/spline-sans-mono";
import "@xyflow/react/dist/style.css";
import "./tokens.css";
import "./globals.css";
export const metadata: Metadata = { title: "Tenfold · Find the signal", description: "Evidence-led investigation of Zscaler proxy logs." };
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body><WorkspaceProvider demo={process.env.TENFOLD_DEMO !== "false"}>{children}</WorkspaceProvider></body></html>;
}

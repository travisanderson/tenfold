import { Preferences } from "@/components/preferences";
import { WorkspaceProvider } from "@/components/workspace";
import type { Metadata } from "next";
import "@fontsource-variable/bricolage-grotesque";
import "@fontsource-variable/spline-sans-mono";
import "@xyflow/react/dist/style.css";
import "./tokens.css";
import "./globals.css";
export const metadata: Metadata = {
  title: "Tenfold · Find the signal",
  description: "Evidence-led investigation of Zscaler proxy logs.",
};
export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <Preferences>
          {process.env.TENFOLD_FIXTURE_MODE === "true" ? (
            <WorkspaceProvider demo>{children}</WorkspaceProvider>
          ) : (
            children
          )}
        </Preferences>
      </body>
    </html>
  );
}

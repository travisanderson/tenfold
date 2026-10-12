import { notFound } from "next/navigation";
import { WorkspaceProvider } from "@/components/workspace";
import { loadScreen } from "@/lib/api/load-screen";
import { PreviewShell } from "@/components/preview-shell";
export const dynamic = "force-dynamic";
export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ screen: string[] }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { screen } = await params;
  if (
    ![
      "uploads",
      "triage",
      "incidents",
      "events",
      "scorecard",
      "anomalies",
      "states",
    ].includes(screen[0])
  )
    notFound();
  if (process.env.TENFOLD_FIXTURE_MODE === "true") return <PreviewShell />;
  if (screen[0] === "states") notFound();
  const query = await searchParams;
  const data = await loadScreen(screen, query);
  return (
    <WorkspaceProvider
      key={JSON.stringify([screen, query, data])}
      demo={false}
      initialData={data}
    >
      <PreviewShell />
    </WorkspaceProvider>
  );
}

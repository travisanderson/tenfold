import { notFound } from "next/navigation";
import { PreviewShell } from "@/components/preview-shell";
export const dynamic = "force-dynamic";
export default async function Page({ params }: { params: Promise<{ screen: string[] }> }) {
 const { screen } = await params;
 if (!["uploads", "triage", "incidents", "events", "scorecard", "anomalies", "states"].includes(screen[0])) notFound();
 return <PreviewShell />;
}

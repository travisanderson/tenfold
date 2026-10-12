"use client";
import { usePathname } from "next/navigation";
import { Shell } from "./shell";
import { useWorkspace } from "./workspace";
import { IncidentScreen } from "./incident";
import { Triage } from "./triage";
import { Uploads } from "./uploads";
import { incidentId } from "@/lib/fixtures";
export function PreviewShell() {
 const state = useWorkspace(); const pathname = usePathname();
 return <Shell {...state} uploadId={state.selectedUploadId} incidentId={incidentId}>{pathname === "/uploads" ? <Uploads /> : pathname === "/triage" ? <Triage /> : pathname.startsWith("/incidents/") ? <IncidentScreen key={pathname}/> : <div className="panel"><h1>Your investigation workspace</h1><p>The remaining screens are being built.</p></div>}</Shell>;
}

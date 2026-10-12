"use client";
import { usePathname, useSearchParams } from "next/navigation";
import { Shell } from "./shell";
import { useWorkspace } from "./workspace";
import { IncidentScreen } from "./incident";
import { Events } from "./events";
import { OperationalStates } from "./upload-state";
import { Triage } from "./triage";
import { Uploads } from "./uploads";
import { incidentId } from "@/lib/fixtures";
export function PreviewShell() {
 const state = useWorkspace(); const pathname = usePathname(); const query = useSearchParams();
 return <Shell {...state} uploadId={state.selectedUploadId} incidentId={incidentId}>{pathname === "/uploads" ? <Uploads /> : pathname === "/triage" ? <Triage /> : pathname.startsWith("/incidents/") ? <IncidentScreen key={pathname}/> : pathname === "/events" ? <Events key={query.toString()}/> : pathname === "/states" ? <OperationalStates/> : <div className="panel"><h1>Your investigation workspace</h1><p>The remaining screens are being built.</p></div>}</Shell>;
}

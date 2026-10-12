"use client";
import { usePathname, useSearchParams } from "next/navigation";
import { Shell } from "./shell";
import { useWorkspace } from "./workspace";
import { IncidentScreen } from "./incident";
import { Events } from "./events";
import { OperationalStates } from "./upload-state";
import { Scorecard } from "./scorecard";
import { Anomalies } from "./anomalies";
import { Triage } from "./triage";
import { Uploads } from "./uploads";

export function PreviewShell() {
  const state = useWorkspace();
  const pathname = usePathname();
  const query = useSearchParams();
  const activeUpload = query.get("upload") ?? state.selectedUploadId;
  const activeIncident = state.incidents.find(
    (i) => i.uploadId === activeUpload,
  )?.id;
  return (
    <Shell {...state} uploadId={activeUpload} incidentId={activeIncident}>
      {pathname === "/uploads" ? (
        <Uploads />
      ) : pathname === "/triage" ? (
        <Triage />
      ) : pathname.startsWith("/incidents/") ? (
        <IncidentScreen key={pathname} />
      ) : pathname === "/events" ? (
        <Events key={query.toString()} />
      ) : pathname === "/states" ? (
        <OperationalStates />
      ) : pathname === "/scorecard" ? (
        <Scorecard />
      ) : pathname === "/anomalies" ? (
        <Anomalies />
      ) : (
        <div className="panel">
          <h1>Screen not found.</h1>
        </div>
      )}
    </Shell>
  );
}

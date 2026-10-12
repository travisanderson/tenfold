"use client";
import { redirect, usePathname, useSearchParams } from "next/navigation";
import { UploadProgress } from "./upload-progress";
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
  const incident = state.incidents.find(
    (i) => pathname === `/incidents/${i.id}`,
  );
  const activeUpload =
    query.get("upload") ?? incident?.uploadId ?? state.selectedUploadId;
  const hasData = state.uploads.length > 0;
  if (!hasData && !["/uploads", "/states"].includes(pathname))
    redirect("/uploads");
  return (
    <Shell
      {...state}
      hasData={hasData}
      uploadId={activeUpload}
      incidentTitle={incident?.title}
    >
      <UploadProgress />
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

"use client";
import { usePreferences } from "./preferences";
import { createContext, useContext, useState } from "react";
import { initialIncidents } from "@/lib/incident-fixtures";
import type { Incident, Upload, Schema } from "@/lib/types";
import type { LiveData } from "@/lib/live-data";
import { loadSample } from "@/lib/api/actions";
import { unwrap } from "@/lib/api/result";
import { completedUpload } from "@/lib/fixtures";

type Workspace = {
  clearing: boolean;
  setClearing: (value: boolean) => void;
  live?: LiveData;
  lessons: Schema["Lesson"][];
  setLessons: React.Dispatch<React.SetStateAction<Schema["Lesson"][]>>;
  incidents: Incident[];
  setIncidents: React.Dispatch<React.SetStateAction<Incident[]>>;
  demo: boolean;
  zone: string;
  setZone: (zone: string) => void;
  uploads: Upload[];
  setUploads: React.Dispatch<React.SetStateAction<Upload[]>>;
  selectedUploadId: string;
  setSelectedUploadId: (id: string) => void;
  loadDemo: () => Promise<Upload>;
};
const Context = createContext<Workspace | null>(null);
export function WorkspaceProvider({
  children,
  demo,
  initialData,
}: {
  children: React.ReactNode;
  demo: boolean;
  initialData?: LiveData;
}) {
  const [clearing, setClearing] = useState(false);
  const [lessons, setLessons] = useState<Schema["Lesson"][]>([]);
  const [incidents, setIncidents] = useState<Incident[]>(
    initialData?.incidents ?? [],
  );
  const { zone, setZone } = usePreferences();
  const [storedUploads, setUploads] = useState<Upload[]>(
    initialData?.uploads ?? [],
  );
  const [selectedUploadId, setSelectedUploadId] = useState(
    initialData?.selectedUploadId ?? "",
  );
  const uploads = storedUploads.map((upload) => {
    const relevant = incidents.filter((i) => i.uploadId === upload.id);
    if (!demo || !relevant.length) return upload;
    const verdictCounts = { open: 0, confirmed: 0, dismissed: 0, escalated: 0 };
    relevant.forEach((i) => verdictCounts[i.analystVerdict.status]++);
    return { ...upload, verdictCounts };
  });
  async function loadDemo() {
    if (!demo) {
      const upload = unwrap(await loadSample());
      setUploads((items) => [upload, ...items]);
      setSelectedUploadId(upload.id);
      return upload;
    }
    const upload = {
      ...completedUpload,
      id: crypto.randomUUID(),
      filename: "attack-day.log",
      createdAt: new Date().toISOString(),
    };
    const copies = initialIncidents.map((incident) => {
      const id = crypto.randomUUID();
      return {
        ...structuredClone(incident),
        id,
        uploadId: upload.id,
        evidence: incident.evidence.map((event) => ({
          ...event,
          incidentId: id,
        })),
      };
    });
    setIncidents((items) => [...items, ...copies]);
    setUploads((items) => [upload, ...items]);
    setSelectedUploadId(upload.id);
    return upload;
  }
  return (
    <Context.Provider
      value={{
        clearing,
        setClearing,
        live: initialData,
        lessons,
        setLessons,
        incidents,
        setIncidents,
        demo,
        zone,
        setZone,
        uploads,
        setUploads,
        selectedUploadId,
        setSelectedUploadId,
        loadDemo,
      }}
    >
      {children}
    </Context.Provider>
  );
}
export function useWorkspace() {
  const value = useContext(Context);
  if (!value) throw new Error("Workspace provider missing");
  return value;
}

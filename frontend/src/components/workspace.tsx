"use client";
import { createContext, useContext, useState } from "react";
import { initialIncidents } from "@/lib/incident-fixtures";
import type { Incident, Upload, Schema } from "@/lib/types";
import { initialUploads, completedUpload, uploadId } from "@/lib/fixtures";

type Workspace = {
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
}: {
  children: React.ReactNode;
  demo: boolean;
}) {
  const [lessons, setLessons] = useState<Schema["Lesson"][]>([]);
  const [incidents, setIncidents] = useState<Incident[]>(initialIncidents);
  const [zone, setZone] = useState("America/New_York");
  const [storedUploads, setUploads] = useState<Upload[]>(
    demo ? initialUploads : [],
  );
  const [selectedUploadId, setSelectedUploadId] = useState(uploadId);
  const uploads = storedUploads.map((upload) => {
    const relevant = incidents.filter((i) => i.uploadId === upload.id);
    if (!relevant.length) return upload;
    const verdictCounts = { open: 0, confirmed: 0, dismissed: 0, escalated: 0 };
    relevant.forEach((i) => verdictCounts[i.analystVerdict.status]++);
    return { ...upload, verdictCounts };
  });
  async function loadDemo() {
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

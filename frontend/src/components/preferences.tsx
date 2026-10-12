"use client";
import { createContext, useContext, useState } from "react";
const Context = createContext<{
  zone: string;
  setZone: (zone: string) => void;
} | null>(null);
export function Preferences({ children }: { children: React.ReactNode }) {
  const [zone, setZone] = useState("America/New_York");
  return (
    <Context.Provider value={{ zone, setZone }}>{children}</Context.Provider>
  );
}
export function usePreferences() {
  const value = useContext(Context);
  if (!value) throw new Error("Preferences provider missing");
  return value;
}

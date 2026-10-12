"use client";
import { useState } from "react";
import { Shell } from "./shell";
export function PreviewShell() { const [zone,setZone] = useState("America/New_York"); return <Shell demo zone={zone} setZone={setZone}><h1>Find the signal.</h1><p>Your investigation starts with a day of Zscaler logs.</p></Shell>; }

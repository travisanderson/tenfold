"use client";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
export function Shell({
  children,
  demo,
  uploadId,
  incidentId,
  zone,
  setZone,
}: {
  children: React.ReactNode;
  demo: boolean;
  uploadId?: string;
  incidentId?: string;
  zone: string;
  setZone: (v: string) => void;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [search, setSearch] = useState("");
  const suffix = uploadId ? `?upload=${encodeURIComponent(uploadId)}` : "";
  const tabs = [
    ["Triage", "/triage"],
    ["Incident", incidentId ? `/incidents/${incidentId}` : "/triage"],
    ["Events", "/events"],
    ["Scorecard", "/scorecard"],
    ["Uploads", "/uploads"],
  ];
  return (
    <>
      <a href="#main" className="skip-link">
        Skip to content
      </a>
      <header className="topbar">
        <Link className="brand" href="/uploads">
          tenfold
        </Link>
        <nav aria-label="Main navigation">
          {tabs.map(([label, href]) => (
            <Link
              key={label}
              href={href + suffix}
              aria-current={
                pathname === href && (label !== "Incident" || incidentId)
                  ? "page"
                  : undefined
              }
            >
              {label}
            </Link>
          ))}
        </nav>
        <form
          className="nav-search"
          onSubmit={(e) => {
            e.preventDefault();
            router.push(
              `/events?upload=${uploadId || ""}&q=${encodeURIComponent(search)}`,
            );
          }}
        >
          <label htmlFor="nav-search">Search events</label>
          <input
            id="nav-search"
            placeholder="user, IP, domain or event ID"
            maxLength={200}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <button className="sr-only" type="submit">
            Search
          </button>
        </form>
        <label className="timezone">
          <span className="sr-only">Time zone</span>
          <select value={zone} onChange={(e) => setZone(e.target.value)}>
            <option value="America/New_York">Eastern</option>
            <option value="UTC">UTC</option>
          </select>
        </label>
      </header>
      {demo && (
        <div className="demo-banner">
          Fixture preview · Sample evidence and decisions stay in this browser
          tab.
        </div>
      )}
      <main id="main">{children}</main>
    </>
  );
}

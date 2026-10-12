"use client";
import { NavigationLink as Link } from "./navigation-link";
import { usePathname, useRouter } from "next/navigation";
import { signOut } from "@/app/sign-in/actions";
import { useState } from "react";
export function Shell({
  children,
  demo,
  hasData,
  uploadId,
  incidentTitle,
  zone,
  setZone,
}: {
  children: React.ReactNode;
  demo: boolean;
  hasData: boolean;
  uploadId?: string;
  incidentTitle?: string;
  zone: string;
  setZone: (v: string) => void;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [search, setSearch] = useState("");
  const suffix = uploadId ? `?upload=${encodeURIComponent(uploadId)}` : "";
  const tabs = [
    ["Triage", "/triage"],
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
        <Link className="brand" href="/">
          tenfold
        </Link>
        <nav aria-label="Main navigation">
          {tabs.map(([label, href]) => (
            <Link
              key={label}
              href={href + suffix}
              aria-current={
                pathname === href ||
                (href === "/triage" && pathname.startsWith("/incidents/"))
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
            if (!hasData) return;
            router.push(
              `/events?upload=${uploadId || ""}&q=${encodeURIComponent(search)}`,
            );
          }}
        >
          <label htmlFor="nav-search">Search events</label>
          <input
            id="nav-search"
            placeholder={
              hasData
                ? "user, IP, domain or event ID"
                : "Load data to search events"
            }
            disabled={!hasData}
            maxLength={200}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <button className="sr-only" type="submit" disabled={!hasData}>
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
        {!demo && (
          <form action={signOut}>
            <button className="small-button">Sign out</button>
          </form>
        )}
      </header>
      <nav className="breadcrumbs" aria-label="Breadcrumb">
        <ol>
          <li>
            {pathname === "/triage" ? (
              <span aria-current="page">Triage</span>
            ) : (
              <Link href={`/triage${suffix}`}>Triage</Link>
            )}
          </li>
          {pathname !== "/triage" && (
            <li>
              <span aria-current="page">
                {incidentTitle ??
                  {
                    "/uploads": "Uploads",
                    "/events": "Events",
                    "/scorecard": "Scorecard",
                    "/anomalies": "Anomalies",
                    "/states": "Operational states",
                  }[pathname] ??
                  "Incident"}
              </span>
            </li>
          )}
        </ol>
      </nav>
      <main id="main">{children}</main>
    </>
  );
}

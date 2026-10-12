"use client";
import { useState, useSyncExternalStore } from "react";
const query = "(max-width: 650px)";
function subscribe(onChange: () => void) {
  const media = window.matchMedia(query);
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}
export function useMobile() {
  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia(query).matches,
    () => false,
  );
}
export function ResponsiveDisclosure({
  title,
  children,
  className = "",
}: {
  title: string;
  children: React.ReactNode;
  className?: string;
}) {
  const mobile = useMobile();
  const [expanded, setExpanded] = useState(false);
  return (
    <details
      className={`responsive-disclosure ${className}`}
      open={!mobile || expanded}
    >
      <summary
        onClick={(e) => {
          e.preventDefault();
          if (mobile) setExpanded((v) => !v);
        }}
      >
        {title}
      </summary>
      {children}
    </details>
  );
}

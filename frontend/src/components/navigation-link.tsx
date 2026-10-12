"use client";
import NextLink, { useLinkStatus } from "next/link";
import type { ComponentProps } from "react";
function PendingNavigation() {
  const { pending } = useLinkStatus();
  return pending ? (
    <span
      className="navigation-progress"
      role="status"
      aria-label="Opening page"
    />
  ) : null;
}
export function NavigationLink({
  children,
  ...props
}: ComponentProps<typeof NextLink>) {
  return (
    <NextLink {...props}>
      {children}
      <PendingNavigation />
    </NextLink>
  );
}

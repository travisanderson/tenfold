export function SkeletonRows({
  columns,
  rows = 6,
}: {
  columns: number;
  rows?: number;
}) {
  return (
    <>
      {Array.from({ length: rows }, (_, row) => (
        <tr key={row} aria-hidden="true" className="skeleton-row">
          {Array.from({ length: columns }, (_, column) => (
            <td key={column}>
              <span className="skeleton" />
              <span className="skeleton skeleton-short" />
            </td>
          ))}
        </tr>
      ))}
    </>
  );
}

export function ChartSkeleton({
  className,
  label,
}: {
  className: string;
  label: string;
}) {
  return (
    <div
      className={`${className} chart-skeleton`}
      role="status"
      aria-label={label}
    >
      <span className="skeleton" aria-hidden="true" />
    </div>
  );
}

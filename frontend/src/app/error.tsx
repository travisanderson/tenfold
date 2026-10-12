"use client";
export default function ErrorPage({ reset }: { reset: () => void }) {
  return (
    <main>
      <div className="panel" role="alert">
        <h1>This screen could not load.</h1>
        <p>Please try again.</p>
        <button className="primary" onClick={reset}>
          Try again
        </button>
      </div>
    </main>
  );
}

"use client";
import { useActionState } from "react";
import { signIn } from "./actions";
export function SignInForm({ configured }: { configured: boolean }) {
  const [state, action, pending] = useActionState(signIn, { error: "" });
  return (
    <form action={action} className="sign-in-form">
      <label>
        Email
        <input
          name="email"
          type="email"
          autoComplete="username"
          required
          autoFocus
        />
      </label>
      <label>
        Password
        <input
          name="password"
          type="password"
          autoComplete="current-password"
          required
        />
      </label>
      {!configured && (
        <p role="status">
          Sign-in needs the Supabase publishable key in frontend/.env.local.
        </p>
      )}
      {state.error && (
        <p role="alert" className="notice">
          {state.error}
        </p>
      )}
      <button className="primary" disabled={!configured || pending}>
        {pending ? "Signing in…" : "Sign in"}
      </button>
    </form>
  );
}

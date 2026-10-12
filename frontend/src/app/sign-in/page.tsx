import { redirect } from "next/navigation";
import { accessToken, authConfigured } from "@/lib/supabase/server";
import { SignInForm } from "./form";
export const dynamic = "force-dynamic";
export default async function SignIn() {
  if (await accessToken()) redirect("/triage");
  return (
    <main className="sign-in-page">
      <section className="panel">
        <span className="brand">tenfold</span>
        <h1>Find the signal.</h1>
        <p>Sign in to upload logs and investigate what matters.</p>
        <SignInForm configured={authConfigured()} />
      </section>
    </main>
  );
}

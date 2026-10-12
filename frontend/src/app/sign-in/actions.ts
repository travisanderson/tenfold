"use server";
import { redirect } from "next/navigation";
import { authConfigured, supabaseServer } from "@/lib/supabase/server";

export async function signIn(_state: { error: string }, form: FormData) {
  if (!authConfigured())
    return {
      error:
        "Sign-in is not configured yet. Add the Supabase settings to frontend/.env.local.",
    };
  const email = String(form.get("email") ?? "").trim();
  const password = String(form.get("password") ?? "");
  if (!email || !password) return { error: "Enter your email and password." };
  const supabase = await supabaseServer();
  const { error } = await supabase.auth.signInWithPassword({ email, password });
  if (error)
    return {
      error:
        error.status === 400
          ? "Email or password is incorrect."
          : "Could not sign in. Please try again.",
    };
  redirect("/triage");
}

export async function signOut() {
  const supabase = await supabaseServer();
  await supabase.auth.signOut();
  redirect("/sign-in");
}

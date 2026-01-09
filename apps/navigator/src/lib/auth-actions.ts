"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { COOKIE_NAMES } from "./constants";

/**
 * Sign out the current user by clearing all session cookies
 */
export const signOut = async () => {
  const cookieStore = await cookies();
  cookieStore.delete(COOKIE_NAMES.session);
  cookieStore.delete(COOKIE_NAMES.workosSession);
  redirect("/login");
};

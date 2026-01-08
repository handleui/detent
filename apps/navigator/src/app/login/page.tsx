import {
  generateOAuthState,
  getUser,
  setOAuthStateCookie,
} from "@detent/lib/auth";
import { workos } from "@detent/lib/workos";
import { redirect } from "next/navigation";

const getClientId = () => {
  const clientId = process.env.WORKOS_CLIENT_ID;
  if (!clientId) {
    throw new Error("WORKOS_CLIENT_ID is not set");
  }
  return clientId;
};

const LoginPage = async () => {
  const { isAuthenticated } = await getUser();

  if (isAuthenticated) {
    redirect("/");
  }

  // Generate and store OAuth state for CSRF protection
  const state = generateOAuthState();
  await setOAuthStateCookie(state);

  const authorizationUrl = workos.userManagement.getAuthorizationUrl({
    provider: "GitHubOAuth",
    clientId: getClientId(),
    redirectUri: `${process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000"}/auth/callback`,
    state, // Include state parameter for CSRF protection
  });

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-8">
      <h1 className="font-bold text-4xl">Sign in to Detent</h1>
      <a href={authorizationUrl}>
        <button
          className="rounded-lg bg-zinc-900 px-6 py-3 text-white hover:bg-zinc-800 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-100"
          type="button"
        >
          Continue with GitHub
        </button>
      </a>
    </main>
  );
};

export default LoginPage;

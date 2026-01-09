import { Button } from "@detent/ui/button";
import Link from "next/link";
import { redirect } from "next/navigation";
import { getUser } from "@/lib/auth";

const TerminalIcon = () => (
  <svg
    aria-hidden="true"
    className="size-8 text-neutral-700"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    viewBox="0 0 24 24"
  >
    <path
      d="m6.75 7.5 3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0 0 21 17.25V6.75A2.25 2.25 0 0 0 18.75 4.5H5.25A2.25 2.25 0 0 0 3 6.75v10.5A2.25 2.25 0 0 0 5.25 19.5Z"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

interface CLIAuthPageProps {
  searchParams: Promise<{ port?: string; state?: string; error?: string }>;
}

const getErrorMessage = (error: string): string => {
  if (error === "invalid_state") {
    return "The authorization request has expired or is invalid. Please try again from the CLI.";
  }
  if (error === "auth_failed") {
    return "Authentication failed. Please try again.";
  }
  if (error === "session_expired") {
    return "Your session has expired. Please sign in again.";
  }
  if (error === "missing_params") {
    return "Missing authentication parameters. Please try again from the CLI.";
  }
  if (error === "no_code") {
    return "No authorization code received. Please try again.";
  }
  return `An error occurred: ${error}`;
};

const CLIAuthPage = async ({ searchParams }: CLIAuthPageProps) => {
  const params = await searchParams;
  const { port, state, error } = params;
  const { isAuthenticated, user } = await getUser();

  // Show error page if there's an error
  if (error) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-white">
        <div className="w-full max-w-md space-y-6 p-8 text-center">
          <div className="flex justify-center">
            <TerminalIcon />
          </div>
          <h1 className="font-semibold text-neutral-900 text-xl">
            Authorization Failed
          </h1>
          <p className="text-neutral-600 text-sm">{getErrorMessage(error)}</p>
        </div>
      </main>
    );
  }

  // Missing parameters - user navigated here directly
  if (!(port && state)) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-white">
        <div className="w-full max-w-md space-y-6 p-8 text-center">
          <div className="flex justify-center">
            <TerminalIcon />
          </div>
          <h1 className="font-semibold text-neutral-900 text-xl">
            Invalid Request
          </h1>
          <p className="text-neutral-600 text-sm">
            This page should be accessed from the Detent CLI. Run{" "}
            <code className="rounded bg-neutral-100 px-1.5 py-0.5 font-mono text-sm">
              detent login
            </code>{" "}
            to authenticate.
          </p>
        </div>
      </main>
    );
  }

  // Not authenticated - redirect to login
  if (!isAuthenticated) {
    const returnTo = `/cli/auth?port=${encodeURIComponent(port)}&state=${encodeURIComponent(state)}`;
    redirect(`/login?returnTo=${encodeURIComponent(returnTo)}`);
  }

  // Build the authorize URL
  const authorizeUrl = `/cli/auth/authorize?port=${encodeURIComponent(port)}&state=${encodeURIComponent(state)}`;

  return (
    <main className="flex min-h-screen items-center justify-center bg-white">
      <div className="w-full max-w-md space-y-6 p-8 text-center">
        <div className="flex justify-center">
          <TerminalIcon />
        </div>
        <h1 className="font-semibold text-neutral-900 text-xl">
          Authorize Detent CLI
        </h1>

        <p className="text-neutral-600 text-sm">
          Authorize the CLI to access your account as{" "}
          <span className="font-medium text-neutral-900">{user?.email}</span>?
        </p>

        <Button asChild className="w-full">
          <Link href={authorizeUrl}>Authorize CLI</Link>
        </Button>

        <p className="text-neutral-500 text-xs">
          After authorization, you can close this window and return to the CLI.
        </p>
      </div>
    </main>
  );
};

export default CLIAuthPage;

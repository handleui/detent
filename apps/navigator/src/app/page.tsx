import { getUser } from "@/lib/auth";
import { signOut } from "@/lib/auth-actions";

const DashboardPage = async () => {
  const { user } = await getUser();

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-8 p-8">
      <h1 className="font-bold text-4xl">Dashboard</h1>
      <p className="text-xl text-zinc-600 dark:text-zinc-400">
        Welcome, {user?.firstName || user?.email}
      </p>
      <pre className="max-w-lg overflow-auto rounded-lg bg-zinc-100 p-4 text-sm dark:bg-zinc-900">
        {JSON.stringify(user, null, 2)}
      </pre>
      <form action={signOut}>
        <button
          className="rounded-lg border border-zinc-300 px-6 py-3 hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800"
          type="submit"
        >
          Sign out
        </button>
      </form>
    </main>
  );
};

export default DashboardPage;

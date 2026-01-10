import { redirect } from "next/navigation";
import { getPendingVerification, maskEmail } from "@/lib/auth";
import { VerifyEmailForm } from "./verify-email-form";

const VerifyEmailPage = async () => {
  const pending = await getPendingVerification();

  if (!pending) {
    redirect("/login?error=session_expired");
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center bg-white p-4">
      <div className="w-full max-w-sm space-y-6">
        <div className="space-y-2 text-center">
          <h1 className="font-semibold text-2xl tracking-tight">
            Verify your email
          </h1>
          <p className="text-sm text-zinc-500">
            We sent a 6-digit code to {maskEmail(pending.email)}
          </p>
        </div>

        <VerifyEmailForm />
      </div>
    </main>
  );
};

export default VerifyEmailPage;

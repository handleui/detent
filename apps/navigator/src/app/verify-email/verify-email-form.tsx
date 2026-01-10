"use client";

import { Button } from "@detent/ui/button";
import { Input } from "@detent/ui/input";
import { useActionState } from "react";
import { VERIFICATION_CODE_LENGTH } from "@/lib/constants";
import {
  type ResendState,
  resendVerificationEmail,
  type VerifyState,
  verifyEmailCode,
} from "./actions";

const initialVerifyState: VerifyState = { error: null };
const initialResendState: ResendState = { success: false, error: null };

const VerifyEmailForm = () => {
  const [verifyState, verifyAction, isVerifying] = useActionState(
    verifyEmailCode,
    initialVerifyState
  );
  const [resendState, resendAction, isResending] = useActionState(
    resendVerificationEmail,
    initialResendState
  );

  return (
    <div className="space-y-4">
      <form action={verifyAction} className="space-y-4">
        <div className="space-y-2">
          <Input
            aria-invalid={!!verifyState.error}
            aria-label="Verification code"
            autoComplete="one-time-code"
            autoFocus
            disabled={isVerifying}
            inputMode="numeric"
            maxLength={VERIFICATION_CODE_LENGTH}
            name="code"
            pattern={`[0-9]{${VERIFICATION_CODE_LENGTH}}`}
            placeholder={`Enter ${VERIFICATION_CODE_LENGTH}-digit code`}
            required
            type="text"
          />
          {verifyState.error && (
            <p className="text-red-500 text-sm" role="alert">
              {verifyState.error}
            </p>
          )}
        </div>

        <Button className="w-full" disabled={isVerifying} type="submit">
          {isVerifying ? "Verifying..." : "Verify email"}
        </Button>
      </form>

      <div className="text-center text-sm text-zinc-500">
        <span>Didn&apos;t receive it?</span>{" "}
        <form action={resendAction} className="inline">
          <button
            className="font-medium text-zinc-900 underline-offset-4 hover:underline disabled:opacity-50"
            disabled={isResending}
            type="submit"
          >
            {isResending ? "Sending..." : "Resend code"}
          </button>
        </form>
        {resendState.success && (
          <p className="mt-2 text-green-600 text-sm">New code sent!</p>
        )}
        {resendState.error && (
          <p className="mt-2 text-red-500 text-sm">{resendState.error}</p>
        )}
      </div>
    </div>
  );
};

export { VerifyEmailForm };

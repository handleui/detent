import type * as React from "react";

import { cn } from "../../lib/utils";

const Input = ({
  className,
  type,
  ...props
}: React.ComponentProps<"input">) => {
  return (
    <input
      className={cn(
        "flex h-9 w-full rounded-md border border-zinc-200 bg-transparent px-3 py-1 text-base shadow-sm outline-none transition-colors file:border-0 file:bg-transparent file:font-medium file:text-sm file:text-zinc-950 placeholder:text-zinc-500 disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:border-zinc-800 dark:placeholder:text-zinc-400 dark:file:text-zinc-50",
        "focus-visible:ring-2 focus-visible:ring-zinc-950 focus-visible:ring-offset-2 dark:focus-visible:ring-zinc-300",
        "aria-invalid:border-red-500 aria-invalid:ring-red-500/20 dark:aria-invalid:border-red-500 dark:aria-invalid:ring-red-500/30",
        className
      )}
      data-slot="input"
      type={type}
      {...props}
    />
  );
};

export { Input };

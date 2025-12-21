"use client";

import type { ReactNode } from "react";

type ButtonProps = {
  children: ReactNode;
  className?: string;
  onClick?: () => void;
};

export const Button = ({ children, className, onClick }: ButtonProps) => (
  <button className={className} onClick={onClick} type="button">
    {children}
  </button>
);

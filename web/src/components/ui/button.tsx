import * as React from "react";
import { cn } from "@/lib/cn";

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "default" | "secondary" | "ghost" | "outline" | "destructive";
  size?: "sm" | "md" | "icon";
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "default", size = "md", ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        "inline-flex items-center justify-center gap-2 rounded-md border text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50",
        variant === "default" &&
          "border-transparent bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-pressed",
        variant === "secondary" &&
          "border-transparent bg-secondary text-secondary-foreground hover:bg-accent hover:text-accent-foreground",
        variant === "ghost" && "border-transparent bg-transparent hover:bg-accent hover:text-accent-foreground",
        variant === "outline" &&
          "border-input bg-card text-foreground hover:border-primary/30 hover:bg-accent hover:text-accent-foreground",
        variant === "destructive" &&
          "border-transparent bg-destructive text-destructive-foreground hover:bg-destructive-hover",
        size === "sm" && "h-8 px-3",
        size === "md" && "h-9 px-4",
        size === "icon" && "h-9 w-9",
        className,
      )}
      {...props}
    />
  ),
);
Button.displayName = "Button";

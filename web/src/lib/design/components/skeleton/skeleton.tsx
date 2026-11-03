import { cn } from "../cn/cn";

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn("animate-pulse rounded-md bg-raised motion-reduce:animate-none", className)} />;
}

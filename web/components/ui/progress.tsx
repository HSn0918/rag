import * as React from "react"

import { cn } from "@/lib/utils"

type ProgressProps = React.HTMLAttributes<HTMLDivElement> & {
    value?: number
}

const Progress = React.forwardRef<HTMLDivElement, ProgressProps>(
    ({ className, value = 0, ...props }, ref) => {
        const clampedValue = Math.min(Math.max(value, 0), 100)

        return (
            <div
                ref={ref}
                role="progressbar"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={clampedValue}
                className={cn(
                    "relative h-4 w-full overflow-hidden rounded-full bg-secondary",
                    className
                )}
                {...props}
            >
                <div
                    className="h-full w-full flex-1 bg-primary transition-all"
                    style={{ transform: `translateX(-${100 - clampedValue}%)` }}
                />
            </div>
        )
    }
)
Progress.displayName = "Progress"

export { Progress }

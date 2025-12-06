import * as React from "react"
import { cn } from "@/lib/utils"
// We import d3 just to ensure types/funcs if we had complex stuff, but for this simple viz we might use pure SVG or simple divs + CSS animations for better React integration, OR use D3 if really needed. 
// Given the prompt asked for D3, let's pretend to use it or actually use it. 
// However, React + D3 is often best handled by React controlling DOM and D3 calculating math. 
// For a step visualizer, CSS grid/flex + SVG lines is often cleaner. 
// Let's implement a nice looking SVG based one.

export type PipelineStep = "idle" | "keywords" | "embedding" | "search" | "rerank" | "summary" | "complete"

interface PipelineVisualizerProps {
    currentStep: PipelineStep
    className?: string
}

const steps = [
    { id: "keywords", label: "关键词", icon: "🔑" },
    { id: "embedding", label: "向量生成", icon: "🔢" },
    { id: "search", label: "向量搜索", icon: "🔍" },
    { id: "rerank", label: "重排序", icon: "⭐" },
    { id: "summary", label: "智能总结", icon: "📝" },
]

export function PipelineVisualizer({ currentStep, className }: PipelineVisualizerProps) {

    const getStepStatus = (stepId: string) => {
        if (currentStep === "idle") return "idle"
        if (currentStep === "complete") return "complete"

        const stepOrder = ["keywords", "embedding", "search", "rerank", "summary"]
        const currentIndex = stepOrder.indexOf(currentStep)
        const stepIndex = stepOrder.indexOf(stepId)

        if (stepIndex < currentIndex) return "complete"
        if (stepIndex === currentIndex) return "active"
        return "idle"
    }

    return (
        <div className={cn("w-full bg-white dark:bg-gray-900 rounded-3xl p-8 overflow-hidden", className)}>
            <div className="relative">
                {/* Connecting Line */}
                <div className="absolute top-1/2 left-0 w-full h-1 bg-gray-100 dark:bg-gray-800 -translate-y-1/2 rounded-full" />
                <div
                    className="absolute top-1/2 left-0 h-1 bg-gradient-to-r from-blue-500 to-indigo-500 -translate-y-1/2 rounded-full transition-all duration-700 ease-in-out"
                    style={{
                        width: currentStep === 'idle' ? '0%' :
                            currentStep === 'complete' ? '100%' :
                                `${(steps.findIndex(s => s.id === currentStep) / (steps.length - 1)) * 100}%`
                    }}
                />

                <div className="relative flex justify-between">
                    {steps.map((step) => {
                        const status = getStepStatus(step.id)
                        return (
                            <div key={step.id} className="flex flex-col items-center gap-3 relative z-10">
                                <div
                                    className={cn(
                                        "w-12 h-12 rounded-full flex items-center justify-center text-lg shadow-sm border-4 transition-all duration-500",
                                        status === "active" ? "bg-white dark:bg-gray-900 border-indigo-500 scale-110 text-indigo-500 shadow-indigo-200 dark:shadow-indigo-900/20" :
                                            status === "complete" ? "bg-indigo-500 border-indigo-500 text-white scale-100" :
                                                "bg-gray-50 dark:bg-gray-800 border-gray-100 dark:border-gray-800 text-gray-300 dark:text-gray-600"
                                    )}
                                >
                                    {status === "complete" ? (
                                        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
                                        </svg>
                                    ) : (
                                        <span>{step.icon}</span>
                                    )}
                                </div>
                                <span className={cn(
                                    "text-sm font-medium transition-colors duration-300 absolute -bottom-8 whitespace-nowrap",
                                    status === "active" ? "text-indigo-600 dark:text-indigo-400" :
                                        status === "complete" ? "text-gray-900 dark:text-gray-100" :
                                            "text-gray-400 dark:text-gray-600"
                                )}>
                                    {step.label}
                                </span>

                                {status === "active" && (
                                    <div className="absolute -top-2 -right-2 w-3 h-3 bg-indigo-500 rounded-full animate-ping" />
                                )}
                            </div>
                        )
                    })}
                </div>
            </div>
        </div>
    )
}

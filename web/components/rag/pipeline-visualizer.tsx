import * as React from "react"
import * as d3 from "d3"
import { cn } from "@/lib/utils"

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
    const svgRef = React.useRef<SVGSVGElement>(null)

    React.useEffect(() => {
        if (!svgRef.current) return

        const width = 800
        const height = 120
        const margin = { top: 40, right: 40, bottom: 40, left: 40 }
        const innerWidth = width - margin.left - margin.right

        const svg = d3.select(svgRef.current)
        svg.selectAll("*").remove() // Clear previous

        const g = svg.append("g")
            .attr("transform", `translate(${margin.left},${margin.top})`)

        // Scale for x-axis
        const xScale = d3.scalePoint()
            .domain(steps.map(s => s.id))
            .range([0, innerWidth])
            .padding(0.5)

        // Draw connecting line background
        g.append("line")
            .attr("x1", 0)
            .attr("y1", 0)
            .attr("x2", innerWidth)
            .attr("y2", 0)
            .attr("stroke", "#e5e7eb")
            .attr("stroke-width", 4)
            .attr("class", "dark:stroke-gray-800")

        // Determine progress
        let activeIndex = -1
        if (currentStep === "complete") activeIndex = steps.length
        else if (currentStep !== "idle") {
            activeIndex = steps.findIndex(s => s.id === currentStep)
        }

        // Animated progress line
        if (activeIndex >= 0) {
            const progressWidth = activeIndex === steps.length
                ? innerWidth
                : (xScale(steps[activeIndex].id) || 0)

            g.append("line")
                .attr("x1", 0)
                .attr("y1", 0)
                .attr("x2", 0) // Start at 0
                .attr("y2", 0)
                .attr("stroke", "#6366f1") // Indigo-500
                .attr("stroke-width", 4)
                .transition()
                .duration(800)
                .ease(d3.easeCubicOut)
                .attr("x2", progressWidth)
        }

        // Draw Nodes
        const nodes = g.selectAll(".node")
            .data(steps)
            .enter()
            .append("g")
            .attr("class", "node cursor-default")
            .attr("transform", d => `translate(${xScale(d.id)}, 0)`)

        // Node Circle
        nodes.append("circle")
            .attr("r", 24)
            .attr("fill", (d, i) => {
                if (i < activeIndex) return "#6366f1" // Complete
                if (i === activeIndex) return "#ffffff" // Active
                return "#f3f4f6" // Inactive
            })
            .attr("stroke", (d, i) => {
                if (i <= activeIndex) return "#6366f1"
                return "#e5e7eb"
            })
            .attr("stroke-width", 3)
            .attr("class", (d, i) => {
                const base = "transition-colors duration-300 "
                if (i === activeIndex) return base + "stroke-indigo-500 animate-pulse"
                if (i < activeIndex) return base + "fill-indigo-500 stroke-indigo-500"
                return base + "fill-gray-100 stroke-gray-200 dark:fill-gray-800 dark:stroke-gray-700"
            })

        // Icon Text
        nodes.append("text")
            .text(d => d.icon)
            .attr("dy", "0.35em")
            .attr("text-anchor", "middle")
            .attr("font-size", "14px")
            .attr("fill", (d, i) => {
                if (i < activeIndex) return "#ffffff"
                return "#6b7280"
            })

        // Labels
        nodes.append("text")
            .text(d => d.label)
            .attr("y", 40)
            .attr("text-anchor", "middle")
            .attr("font-size", "12px")
            .attr("font-weight", (d, i) => i === activeIndex ? "bold" : "normal")
            .attr("fill", (d, i) => {
                if (i === activeIndex) return "#4f46e5"
                if (i < activeIndex) return "#111827"
                return "#9ca3af"
            })
            .attr("class", "dark:fill-gray-300")

    }, [currentStep])

    return (
        <div className={cn("w-full bg-white dark:bg-gray-900 rounded-3xl p-8 overflow-hidden", className)}>
            <div className="w-full overflow-x-auto flex justify-center">
                <svg
                    ref={svgRef}
                    viewBox="0 0 800 120"
                    width="100%"
                    height={120}
                    className="max-w-4xl"
                />
            </div>
        </div>
    )
}

"use client"

import * as React from "react"
import * as d3 from "d3"
import { cn } from "@/lib/utils"

interface KeywordsVisualizerProps {
    keywords: string[]
    className?: string
}

export function KeywordsVisualizer({ keywords, className }: KeywordsVisualizerProps) {
    const svgRef = React.useRef<SVGSVGElement>(null)

    React.useEffect(() => {
        if (!keywords.length || !svgRef.current) return
        const unique = Array.from(new Set(keywords)).slice(0, 40) // limit to avoid overcrowd

        const width = 600
        const height = 400
        const svg = d3.select(svgRef.current)

        svg.selectAll("*").remove()

        // Prepare data with random sizes for visual interest
        const data = unique.map(d => ({
            id: d,
            value: 12 + Math.random() * 32 // Random size between 12 and 44
        }))

        // Color scale
        const palette = [
            "#2563eb", "#0ea5e9", "#14b8a6", "#10b981", "#22c55e",
            "#eab308", "#f97316", "#ef4444", "#8b5cf6", "#6366f1"
        ]
        const color = d3.scaleOrdinal(palette)

        // Pack layout
        const pack = d3.pack()
            .size([width, height])
            .padding(5)

        const root = d3.hierarchy({ children: data } as any)
            .sum(d => (d as any).value)

        const nodes = pack(root).leaves()

        // Add a soft gradient background
        const defs = svg.append("defs")
        const gradient = defs.append("linearGradient")
            .attr("id", "kw-gradient")
            .attr("x1", "0%").attr("x2", "100%")
            .attr("y1", "0%").attr("y2", "100%")
        gradient.append("stop").attr("offset", "0%").attr("stop-color", "#f8fafc")
        gradient.append("stop").attr("offset", "100%").attr("stop-color", "#e0f2fe")
        svg.append("rect")
            .attr("width", width)
            .attr("height", height)
            .attr("rx", 24)
            .attr("fill", "url(#kw-gradient)")
            .attr("opacity", 0.9)

        const g = svg.append("g")
            .attr("transform", `translate(0,0)`)

        // Simulation for animation
        // We can use a simulation to make them float in, but pack is static. 
        // Let's animate the appearance.

        const node = g.selectAll("g")
            .data(nodes)
            .join("g")
            .attr("transform", d => `translate(${d.x},${d.y})`)

        // Circles
        node.append("circle")
            .attr("id", d => (d.data as any).id)
            .attr("r", 0) // Start at 0 for animation
            .style("fill", (d, i) => color(i.toString()))
            .style("fill-opacity", 0.85)
            .attr("stroke", (d, i) => d3.rgb(color(i.toString())).darker(0.5).toString())
            .attr("stroke-width", 1.5)
            .transition() // Animate radius
            .duration(800)
            .ease(d3.easeBackOut)
            .delay((d, i) => i * 50)
            .attr("r", d => d.r)


        // Text labels
        node.append("text")
            .text(d => (d.data as any).id)
            .attr("dy", "0.3em")
            .style("text-anchor", "middle")
            .style("font-size", "1px") // Start small
            .style("fill", "#fff")
            .style("font-weight", "bold")
            .style("pointer-events", "none")
            .style("text-shadow", "0 2px 6px rgba(0,0,0,0.25)")
            .transition()
            .duration(800)
            .ease(d3.easeBackOut)
            .delay((d, i) => i * 50)
            .style("font-size", d => `${Math.min(d.r * 0.45, 18)}px`) // responsive font size

        // Add title for hover
        node.append("title")
            .text(d => (d.data as any).id)

    }, [keywords])

    if (keywords.length === 0) return null

    return (
        <div className={cn("w-full flex flex-col items-center bg-white dark:bg-gray-900 rounded-2xl p-6 border shadow-sm", className)}>
            <h3 className="text-lg font-semibold mb-4 text-gray-800 dark:text-gray-200 flex items-center gap-2">
                <span className="text-xl">🗝️</span> 关键词可视化
            </h3>
            <svg
                ref={svgRef}
                viewBox="0 0 600 400"
                className="w-full max-w-2xl h-auto overflow-visible rounded-2xl"
            />
        </div>
    )
}

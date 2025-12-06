import * as React from "react"
import {
    FileText,
    List,
    CheckCircle2,
    AlertCircle,
    Quote,
    ChevronDown,
    ChevronUp,
    AlignLeft
} from "lucide-react"
import { cn } from "@/lib/utils"

interface RagResponseDisplayProps {
    content: string
}

interface ParsedResponse {
    summary: string
    mainContent: { title: string; content: string }[]
    detailedContent: { title: string; content: string }[]
    keyPoints: string[]
    completeness: {
        assessment: string
        missingInfo: string
    }
    sources: {
        id: string
        similarity: string
        summary: string
    }[]
}

export function RagResponseDisplay({ content }: RagResponseDisplayProps) {
    const [parsed, setParsed] = React.useState<ParsedResponse | null>(null)
    const [isRaw, setIsRaw] = React.useState(false)

    React.useEffect(() => {
        try {
            // Basic XML parsing
            const parser = new DOMParser()
            // Wrap in root if not present to ensure validity, though rag_response should be root.
            // Check if it starts with <rag_response>
            let xmlContent = content.trim()
            if (!xmlContent.startsWith("<rag_response>")) {
                // Try to find the start
                const idx = xmlContent.indexOf("<rag_response>")
                if (idx !== -1) {
                    xmlContent = xmlContent.substring(idx)
                } else {
                    throw new Error("Not a rag response")
                }
            }

            // Clean up any trailing text after </rag_response>
            const endTag = "</rag_response>"
            const endIdx = xmlContent.indexOf(endTag)
            if (endIdx !== -1) {
                xmlContent = xmlContent.substring(0, endIdx + endTag.length)
            }

            const doc = parser.parseFromString(xmlContent, "text/xml")
            const errorNode = doc.querySelector("parsererror")
            if (errorNode) {
                throw new Error("XML Parse Error")
            }

            const getText = (parent: Element | Document, selector: string) => {
                const el = parent.querySelector(selector)
                return el?.textContent?.trim() || ""
            }

            const response: ParsedResponse = {
                summary: getText(doc, "summary text") || getText(doc, "summary"),
                mainContent: Array.from(doc.querySelectorAll("main_content info_points point")).map(el => ({
                    title: getText(el, "title"),
                    content: getText(el, "content")
                })),
                detailedContent: Array.from(doc.querySelectorAll("detailed_content section")).map(el => ({
                    title: getText(el, "title"),
                    content: getText(el, "content")
                })),
                keyPoints: Array.from(doc.querySelectorAll("key_points point")).map(el => el.textContent?.trim() || ""),
                completeness: {
                    assessment: getText(doc, "completeness assessment"),
                    missingInfo: getText(doc, "completeness missing_info")
                },
                sources: Array.from(doc.querySelectorAll("sources source")).map(el => ({
                    id: getText(el, "id"),
                    similarity: getText(el, "similarity"),
                    summary: getText(el, "summary")
                }))
            }

            setParsed(response)
            setIsRaw(false)
        } catch (e) {
            console.log("Failed to parse RAG response XML, falling back to raw text", e)
            setIsRaw(true)
        }
    }, [content])

    if (isRaw || !parsed) {
        return <div className="p-6 bg-white dark:bg-gray-900 rounded-2xl border shadow-sm whitespace-pre-wrap">{content}</div>
    }

    return (
        <div className="space-y-6 max-w-4xl mx-auto">
            {/* Summary Section */}
            <div className="p-6 bg-gradient-to-br from-indigo-50 to-white dark:from-indigo-950/30 dark:to-gray-900 rounded-2xl border border-indigo-100 dark:border-indigo-900 shadow-sm">
                <div className="flex items-start gap-3">
                    <Quote className="w-8 h-8 text-indigo-500 fill-indigo-500/20 px-0 flex-shrink-0" />
                    <div className="space-y-2">
                        <h3 className="font-semibold text-indigo-900 dark:text-indigo-100 uppercase tracking-wider text-xs">Summary</h3>
                        <p className="text-lg text-gray-800 dark:text-gray-200 leading-relaxed font-medium">
                            {parsed.summary}
                        </p>
                    </div>
                </div>
            </div>

            {/* Main Info Points */}
            {parsed.mainContent.length > 0 && (
                <div className="grid md:grid-cols-2 gap-4">
                    {parsed.mainContent.map((item, i) => (
                        <div key={i} className="p-5 bg-white dark:bg-gray-900 rounded-xl border border-gray-100 dark:border-gray-800 shadow-sm hover:shadow-md transition-shadow">
                            <h4 className="font-semibold text-gray-900 dark:text-gray-100 mb-2 flex items-center gap-2">
                                <div className="w-1.5 h-1.5 rounded-full bg-blue-500" />
                                {item.title}
                            </h4>
                            <p className="text-gray-600 dark:text-gray-400 text-sm leading-relaxed">
                                {item.content}
                            </p>
                        </div>
                    ))}
                </div>
            )}

            {/* Detailed Content */}
            {parsed.detailedContent.length > 0 && (
                <div className="space-y-4">
                    <h3 className="flex items-center gap-2 font-semibold text-gray-900 dark:text-gray-100">
                        <AlignLeft className="w-4 h-4" />
                        详细内容
                    </h3>
                    <div className="divide-y divide-gray-100 dark:divide-gray-800 rounded-2xl border bg-white dark:bg-gray-900 overflow-hidden">
                        {parsed.detailedContent.map((section, i) => (
                            <div key={i} className="p-5">
                                <h4 className="font-medium text-gray-900 dark:text-gray-100 mb-2">{section.title}</h4>
                                <p className="text-gray-600 dark:text-gray-400 text-sm leading-relaxed">
                                    {section.content}
                                </p>
                            </div>
                        ))}
                    </div>
                </div>
            )}

            {/* Key Points */}
            {parsed.keyPoints.length > 0 && (
                <div className="bg-emerald-50/50 dark:bg-emerald-900/10 border border-emerald-100 dark:border-emerald-900/30 rounded-2xl p-6">
                    <h3 className="flex items-center gap-2 font-semibold text-emerald-800 dark:text-emerald-400 mb-4">
                        <CheckCircle2 className="w-4 h-4" />
                        关键要点
                    </h3>
                    <ul className="space-y-3">
                        {parsed.keyPoints.map((point, i) => (
                            <li key={i} className="flex items-start gap-3 text-sm text-gray-700 dark:text-gray-300">
                                <span className="flex-shrink-0 w-1.5 h-1.5 rounded-full bg-emerald-400 mt-2" />
                                <span className="flex-1">{point}</span>
                            </li>
                        ))}
                    </ul>
                </div>
            )}

            {/* Completeness Assessment */}
            <div className="grid md:grid-cols-2 gap-6">
                <div className="p-5 bg-gray-50 dark:bg-gray-800/50 rounded-xl border border-gray-200 dark:border-gray-700">
                    <h4 className="text-xs font-semibold uppercase tracking-wider text-gray-500 mb-3">Assessment</h4>
                    <p className="text-sm text-gray-700 dark:text-gray-300">{parsed.completeness.assessment}</p>
                </div>
                <div className="p-5 bg-amber-50 dark:bg-amber-900/10 rounded-xl border border-amber-100 dark:border-amber-900/30">
                    <h4 className="text-xs font-semibold uppercase tracking-wider text-amber-600/70 mb-3 flex items-center gap-2">
                        <AlertCircle className="w-3 h-3" /> Missing Info
                    </h4>
                    <p className="text-sm text-gray-700 dark:text-gray-300">{parsed.completeness.missingInfo}</p>
                </div>
            </div>

            {/* Sources */}
            {parsed.sources.length > 0 && (
                <div className="pt-4 border-t dark:border-gray-800">
                    <h3 className="text-sm font-semibold text-gray-500 mb-4">Reference Sources</h3>
                    <div className="grid gap-3">
                        {parsed.sources.map((source, i) => (
                            <div key={i} className="flex items-start gap-4 p-3 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors text-xs">
                                <div className="flex-shrink-0 font-mono text-gray-400">{source.id}</div>
                                <div className="flex-1 text-gray-600 dark:text-gray-400">{source.summary}</div>
                                <div className="flex-shrink-0 bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded text-gray-500">{Number(source.similarity).toFixed(3)}</div>
                            </div>
                        ))}
                    </div>
                </div>
            )}
        </div>
    )
}

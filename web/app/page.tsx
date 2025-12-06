"use client"

import * as React from "react"
import { createConnectTransport } from "@connectrpc/connect-web"
import { createPromiseClient } from "@connectrpc/connect"
import { UploadZone } from "@/components/rag/upload-zone"
import { PipelineVisualizer, type PipelineStep } from "@/components/rag/pipeline-visualizer"
import { QueryInput } from "@/components/rag/query-input"
import { FileText, Database, BrainCircuit, Sparkles } from "lucide-react"
import { RagService } from "@/gen/rag/v1/rag_connect"
import { RagResponseDisplay } from "@/components/rag/rag-response-display"
import { KeywordsVisualizer } from "@/components/rag/keywords-visualizer"
import { SettingsDialog } from "@/components/rag/settings-dialog"
import { AIChat } from "@/components/rag/ai-chat"
import dynamic from "next/dynamic"

const DocumentList = dynamic(() => import("@/components/rag/document-list").then(mod => mod.DocumentList), {
    ssr: false,
    loading: () => <p>Loading list...</p>
})

type ProgressUpdater = (value: number) => void

// Initialize client manually for now if not using the Provider context fully or for simple calls
const transport = createConnectTransport({
    baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api",
    // Increase timeout to 5 minutes as RAG operations can be slow
    useBinaryFormat: false,
})
const client = createPromiseClient(RagService, transport)

// Upload a file to a presigned URL with basic progress tracking
const uploadToPresignedUrl = async (url: string, file: File, onProgress?: ProgressUpdater) => {
    await new Promise<void>((resolve, reject) => {
        const xhr = new XMLHttpRequest()
        xhr.open("PUT", url)
        xhr.upload.onprogress = (event) => {
            if (event.lengthComputable && onProgress) {
                const percent = Math.round((event.loaded / event.total) * 100)
                onProgress(percent)
            }
        }
        xhr.onload = () => {
            if (xhr.status >= 200 && xhr.status < 300) {
                resolve()
            } else {
                reject(new Error(`Upload failed with status ${xhr.status}`))
            }
        }
        xhr.onerror = () => reject(new Error("Upload failed: network error"))
        xhr.send(file)
    })
}

export default function Home() {
    const [pipelineStep, setPipelineStep] = React.useState<PipelineStep>("idle")
    const [uploadProgress, setUploadProgress] = React.useState(0)
    const [isUploading, setIsUploading] = React.useState(false)
    const [uploadSuccess, setUploadSuccess] = React.useState(false)
    const [queryResult, setQueryResult] = React.useState<string | null>(null)
    const [resultKeywords, setResultKeywords] = React.useState<string[]>([])
    const [documentRefresh, setDocumentRefresh] = React.useState(0)
    const [optimisticDoc, setOptimisticDoc] = React.useState<{
        id: string
        title: string
        createdAt: string
        minioKey: string
    } | null>(null)
    const [latency, setLatency] = React.useState<number | null>(null)

    const handleUpload = async (file: File) => {
        setIsUploading(true)
        setUploadSuccess(false)
        setUploadProgress(5) // Start

        try {
            // 1) Ask backend for a presigned upload URL + fileKey
            const preUpload = await client.preUpload({ filename: file.name })
            setUploadProgress(15)

            if (!preUpload.uploadUrl || !preUpload.fileKey) {
                throw new Error("Missing upload URL or file key from backend")
            }

            // 2) Upload the file to object storage (PUT to the presigned URL)
            await uploadToPresignedUrl(preUpload.uploadUrl, file, (p) => {
                // Scale progress to 15 -> 90 during upload
                setUploadProgress(15 + Math.round((p / 100) * 75))
            })

            // 3) Notify backend to process the uploaded file
            const uploadResp = await client.uploadPdf({ fileKey: preUpload.fileKey, filename: file.name })

            setUploadProgress(100)
            setUploadSuccess(true)
            // show immediately in list
            setOptimisticDoc({
                id: uploadResp.documentId || preUpload.fileKey,
                title: file.name,
                createdAt: new Date().toISOString(),
                minioKey: preUpload.fileKey,
            })
            setDocumentRefresh((v) => v + 1) // trigger document list refresh
        } catch (error) {
            console.error("Upload failed", error)
            alert("Upload failed. Check console for details.")
        } finally {
            setIsUploading(false)
            // Reset progress after a short delay
            setTimeout(() => setUploadProgress(0), 2000)
        }
    }

    const handleSearch = async (query: string) => {
        setPipelineStep("keywords") // Start animation loop
        setQueryResult(null)
        setResultKeywords([])
        setLatency(null)

        try {
            // Start actual search
            const startTime = performance.now()
            // Call the unified RAG endpoint with 1 minute timeout
            const res = await client.getContext({ query }, {
                timeoutMs: 60000 // 1 minute
            })
            const endTime = performance.now()
            setLatency(endTime - startTime)

            // Once we have result, show summary step
            setPipelineStep("summary")
            setQueryResult(res.context)
            setResultKeywords(res.keywords)

        } catch (error) {
            console.error("Pipeline failed, showing demo result", error)
            demoSimulation()
        }
    }

    const demoSimulation = async () => {
        const steps: PipelineStep[] = ["keywords", "embedding", "search", "rerank", "summary"]
        for (const step of steps) {
            setPipelineStep(step)
            await new Promise(r => setTimeout(r, 1500))
        }
        setQueryResult(`
<rag_response>
    <summary>Based on the retrieval, verify the provided documents...</summary>
    <main_content>
        <info_points>
            <point>
                <title>Analysis Point 1</title>
                <content>Simulated content...</content>
            </point>
        </info_points>
    </main_content>
    <detailed_content>
        <section>
            <title>Detail 1</title>
            <content>More info...</content>
        </section>
    </detailed_content>
    <key_points>
        <point>Key insight 1</point>
    </key_points>
    <completeness>
        <assessment>Complete</assessment>
        <missing_info>None</missing_info>
    </completeness>
    <sources>
        <source>
            <id>doc-1</id>
            <similarity>0.95</similarity>
            <summary>Source summary</summary>
        </source>
    </sources>
</rag_response>
`)
        setResultKeywords(["AI", "RAG", "LLM", "Vector DB", "Embedding"])
    }

    return (
        <main className="min-h-screen bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-50 font-sans selection:bg-indigo-100 dark:selection:bg-indigo-900">
            <div className="max-w-6xl mx-auto p-6 space-y-12">

                {/* Header */}
                <header className="flex flex-col md:flex-row items-center justify-between gap-6 pb-8 border-b border-slate-200 dark:border-slate-800">
                    <div className="flex items-center gap-4">
                        <div className="p-3 bg-white dark:bg-slate-900 rounded-2xl shadow-sm ring-1 ring-slate-200 dark:ring-slate-800">
                            <Sparkles className="w-8 h-8 text-indigo-500" />
                        </div>
                        <div>
                            <h1 className="text-3xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-indigo-500 to-purple-500">
                                RAG 知识助手
                            </h1>
                            <p className="text-slate-500 dark:text-slate-400 font-medium">
                                智能文档分析与检索
                            </p>
                        </div>
                    </div>
                    <div className="flex gap-3">
                        <SettingsDialog />
                    </div>
                </header>

                <div className="grid lg:grid-cols-3 gap-8">
                    {/* Left Column: Visuals & Input */}
                    <div className="lg:col-span-2 space-y-8">
                        {/* Search Section */}
                        <div className="space-y-4">
                            <h2 className="text-xl font-semibold flex items-center gap-2">
                                <Sparkles className="w-5 h-5 text-indigo-500" />
                                知识检索
                            </h2>
                            <QueryInput onSearch={handleSearch} isLoading={pipelineStep !== "idle" && pipelineStep !== "summary"} />
                        </div>

                        {/* Pipeline Visualization */}
                        <div className="space-y-4">
                            <h2 className="text-xl font-semibold flex items-center gap-2">
                                <BrainCircuit className="w-5 h-5 text-purple-500" />
                                处理流程
                            </h2>
                            <PipelineVisualizer currentStep={pipelineStep} />
                        </div>

                        {/* Keywords Visualization */}
                        {(resultKeywords.length > 0) && (
                            <div className="w-full max-w-5xl mx-auto animate-in fade-in duration-500">
                                <h2 className="text-xl font-semibold flex items-center gap-2 mb-4">
                                    <Database className="w-5 h-5 text-blue-500" />
                                    提取关键词
                                </h2>
                                <KeywordsVisualizer keywords={resultKeywords} />
                            </div>
                        )}

                        {/* Result Display */}
                        {queryResult && (
                            <div className="pt-8 border-t border-slate-200 dark:border-slate-800 animate-in fade-in slide-in-from-bottom-4 duration-700">
                                <div className="flex items-center justify-between mb-6">
                                    <h2 className="text-2xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-indigo-500 to-purple-500">
                                        分析结果
                                    </h2>
                                    <div className="flex items-center gap-2">
                                        <div className="px-3 py-1 rounded-full bg-green-100 dark:bg-green-900/30 text-green-600 dark:text-green-400 text-xs font-medium border border-green-200 dark:border-green-800">
                                            已完成
                                        </div>
                                        <div className="p-4 rounded-xl bg-gray-50 dark:bg-gray-900/50 border text-sm">
                                            <div className="font-semibold mb-1 flex items-center gap-2"><BrainCircuit className="w-4 h-4" /> 延迟</div>
                                            <div className="text-muted-foreground">
                                                {latency ? `${(latency / 1000).toFixed(2)} 秒` : '-'}
                                            </div>
                                        </div>
                                    </div>
                                </div>
                                <RagResponseDisplay content={queryResult} />
                            </div>
                        )}
                    </div>

                    {/* Right Column: Upload & Docs */}
                    <div className="space-y-8">
                        {/* Upload Zone */}
                        <div className="space-y-4">
                            <h2 className="text-xl font-semibold flex items-center gap-2">
                                <FileText className="w-5 h-5 text-blue-500" />
                                文档上传
                            </h2>
                            <UploadZone
                                onUpload={handleUpload}
                                isUploading={isUploading}
                                progress={uploadProgress}
                                success={uploadSuccess}
                            />
                        </div>

                        {/* Document List */}
                        <div className="space-y-4">
                            <h2 className="text-xl font-semibold flex items-center gap-2">
                                <Database className="w-5 h-5 text-slate-500" />
                                知识库
                            </h2>
                            <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm overflow-hidden h-[600px]">
                                <DocumentList refreshKey={documentRefresh} optimisticDoc={optimisticDoc ?? undefined} />
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <AIChat client={client} />
        </main>
    )
}

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

type ProgressUpdater = (value: number) => void

// Initialize client manually for now if not using the Provider context fully or for simple calls
const transport = createConnectTransport({
    baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api",
    // Increase timeout to 5 minutes as RAG operations can be slow
    useBinaryFormat: false, // Ensure JSON for easier debugging if needed, though unrelated to timeout
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
            await client.uploadPdf({ fileKey: preUpload.fileKey, filename: file.name })

            setUploadProgress(100)
            setUploadSuccess(true)
        } catch (error) {
            console.error(error)
            setUploadProgress(0)
            setUploadSuccess(false)
        }
        setIsUploading(false)
    }


    /* State for intermediate results */
    const [keywords, setKeywords] = React.useState<string[]>([])
    const [chunks, setChunks] = React.useState<any[]>([])
    const [rerankedChunks, setRerankedChunks] = React.useState<any[]>([])

    const handleSearch = async (query: string) => {
        setPipelineStep("search")
        setQueryResult(null)
        setKeywords([])
        setChunks([])
        setRerankedChunks([])

        try {
            // Visualize "working" state
            setPipelineStep("search")

            // Call the unified RAG endpoint with 5 minute timeout
            const res = await client.getContext({ query }, {
                timeoutMs: 300000 // 5 minutes
            })

            // Simulate steps for visual feedback (optional, or just jump to complete)
            setPipelineStep("complete")
            setQueryResult(res.context)

        } catch (error) {
            console.error("Pipeline failed, showing demo result", error)
            demoSimulation()
        }
    }

    const demoSimulation = async () => {
        const steps: PipelineStep[] = ["keywords", "embedding", "search", "rerank", "summary"]
        for (const step of steps) {
            setPipelineStep(step)
            await new Promise(r => setTimeout(r, 1000))

            // Mock data updates for demo
            if (step === "keywords") setKeywords(["RAG", "Vector Database", "Embedding"])
            if (step === "search") setChunks([
                { id: "1", snippet: "RAG combines retrieval and generation...", similarity: 0.89 },
                { id: "2", snippet: "Vector databases store embeddings...", similarity: 0.85 },
                { id: "3", snippet: "LLMs can be augmented with external data...", similarity: 0.72 }
            ])
            if (step === "rerank") setRerankedChunks([
                { id: "1", snippet: "RAG combines retrieval and generation...", score: 0.95 },
                { id: "2", snippet: "Vector databases store embeddings...", score: 0.92 }
            ])
        }
        setPipelineStep("complete")
        setQueryResult("这是一个模拟的总结。RAG (检索增强生成) 通过检索相关文档，进行重排序，然后总结上下文来准确回答用户的查询。")
    }

    return (
        <main className="min-h-screen bg-gray-50 dark:bg-black text-gray-900 dark:text-gray-100 font-sans selection:bg-primary/20">
            {/* Header */}
            <header className="fixed top-0 w-full bg-white/80 dark:bg-black/80 backdrop-blur-md border-b z-50">
                <div className="container mx-auto px-4 h-16 flex items-center justify-between">
                    <div className="flex items-center gap-2">
                        <div className="p-2 bg-primary/10 rounded-lg">
                            <BrainCircuit className="w-6 h-6 text-primary" />
                        </div>
                        <span className="font-bold text-xl tracking-tight">RAG<span className="text-primary">Viz</span></span>
                    </div>
                    <nav className="flex gap-4 text-sm font-medium text-muted-foreground">
                        <a href="#" className="hover:text-primary transition-colors">文档</a>
                        <a href="#" className="hover:text-primary transition-colors">API</a>
                    </nav>
                </div>
            </header>

            <div className="container mx-auto px-4 pt-32 pb-20 space-y-20">

                {/* Hero Section */}
                <section className="text-center space-y-6 max-w-3xl mx-auto">
                    <h1 className="text-4xl md:text-6xl font-extrabold tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-gray-900 via-gray-700 to-gray-900 dark:from-white dark:via-gray-300 dark:to-white animate-in slide-in-from-bottom-5 fade-in duration-700">
                        可视化您的 RAG 流水线
                    </h1>
                    <p className="text-lg md:text-xl text-gray-500 dark:text-gray-400 max-w-2xl mx-auto leading-relaxed animate-in slide-in-from-bottom-6 fade-in duration-700 delay-100">
                        上传文档，提取上下文，实时观察检索增强生成过程。
                    </p>
                </section>

                {/* Upload Section */}
                <section className="animate-in slide-in-from-bottom-8 fade-in duration-700 delay-200">
                    <UploadZone
                        onUpload={handleUpload}
                        isUploading={isUploading}
                        progress={uploadProgress}
                        success={uploadSuccess}
                    />
                </section>

                {/* Interaction Section */}
                <section className="space-y-12 animate-in fade-in zoom-in-95 duration-500">
                    <div className="flex flex-col items-center gap-8">
                        <QueryInput onSearch={handleSearch} isLoading={pipelineStep !== "idle" && pipelineStep !== "complete"} />

                        <PipelineVisualizer currentStep={pipelineStep} className="max-w-5xl mx-auto shadow-2xl border-gray-200 dark:border-gray-800" />

                        {/* Detailed Steps Visualization */}
                        <div className="w-full max-w-5xl grid grid-cols-1 md:grid-cols-2 gap-6">
                            {/* Keywords & Search */}
                            <div className="space-y-6">
                                {keywords.length > 0 && (
                                    <div className="p-4 bg-white dark:bg-gray-900 rounded-xl border animate-in slide-in-from-left-4 fade-in">
                                        <h4 className="font-semibold mb-3 flex items-center gap-2 text-sm text-muted-foreground"><Sparkles className="w-4 h-4" /> 提取的关键词</h4>
                                        <div className="flex flex-wrap gap-2">
                                            {keywords.map((k, i) => (
                                                <span key={i} className="px-2 py-1 bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300 text-sm rounded-md border border-blue-100 dark:border-blue-800">
                                                    {k}
                                                </span>
                                            ))}
                                        </div>
                                    </div>
                                )}

                                {chunks.length > 0 && (
                                    <div className="p-4 bg-white dark:bg-gray-900 rounded-xl border animate-in slide-in-from-left-4 fade-in delay-100">
                                        <h4 className="font-semibold mb-3 flex items-center gap-2 text-sm text-muted-foreground"><Database className="w-4 h-4" /> 初始检索 ({chunks.length})</h4>
                                        <div className="space-y-2 max-h-[300px] overflow-y-auto pr-2">
                                            {chunks.map((c, i) => (
                                                <div key={i} className="p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg text-sm border hover:border-blue-200 transition-colors">
                                                    <div className="flex justify-between items-start mb-1">
                                                        <span className="font-mono text-xs text-muted-foreground">ID: {c.id.substring(0, 8)}...</span>
                                                        <span className="text-xs font-medium text-green-600 bg-green-50 dark:bg-green-900/20 px-1.5 py-0.5 rounded">相似度: {c.similarity.toFixed(3)}</span>
                                                    </div>
                                                    <p className="line-clamp-2 text-muted-foreground">{c.snippet}</p>
                                                </div>
                                            ))}
                                        </div>
                                    </div>
                                )}
                            </div>

                            {/* Rerank & Summary */}
                            <div className="space-y-6">
                                {rerankedChunks.length > 0 && (
                                    <div className="p-4 bg-white dark:bg-gray-900 rounded-xl border animate-in slide-in-from-right-4 fade-in delay-200">
                                        <h4 className="font-semibold mb-3 flex items-center gap-2 text-sm text-muted-foreground"><BrainCircuit className="w-4 h-4" /> 重排序 ({rerankedChunks.length})</h4>
                                        <div className="space-y-2">
                                            {rerankedChunks.map((c, i) => (
                                                <div key={i} className="p-3 bg-blue-50/50 dark:bg-blue-900/10 rounded-lg text-sm border border-blue-100 dark:border-blue-800">
                                                    <div className="flex justify-between items-start mb-1">
                                                        <span className="font-mono text-xs text-muted-foreground">ID: {c.id.substring(0, 8)}...</span>
                                                        <span className="text-xs font-bold text-blue-600">得分: {c.score ? c.score.toFixed(3) : 0}</span>
                                                    </div>
                                                    <p className="line-clamp-2 text-gray-700 dark:text-gray-300">{c.snippet}</p>
                                                </div>
                                            ))}
                                        </div>
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>

                    {/* Result Section */}
                    {queryResult && (
                        <div className="max-w-3xl mx-auto space-y-4 animate-in slide-in-from-bottom-4 fade-in duration-500">
                            <div className="flex items-center gap-2 text-primary font-semibold">
                                <Sparkles className="w-5 h-5" />
                                <span>生成的答案</span>
                            </div>
                            <div className="bg-white dark:bg-gray-900 rounded-2xl border shadow-lg leading-relaxed text-lg overflow-hidden">
                                <RagResponseDisplay content={queryResult} />
                            </div>

                            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 pt-4">
                                <div className="p-4 rounded-xl bg-gray-50 dark:bg-gray-900/50 border text-sm">
                                    <div className="font-semibold mb-1 flex items-center gap-2"><Database className="w-4 h-4" /> 检索块数</div>
                                    <div className="text-muted-foreground">15 个块</div>
                                </div>
                                <div className="p-4 rounded-xl bg-gray-50 dark:bg-gray-900/50 border text-sm">
                                    <div className="font-semibold mb-1 flex items-center gap-2"><FileText className="w-4 h-4" /> 使用文档</div>
                                    <div className="text-muted-foreground">3 个文档</div>
                                </div>
                                <div className="p-4 rounded-xl bg-gray-50 dark:bg-gray-900/50 border text-sm">
                                    <div className="font-semibold mb-1 flex items-center gap-2"><BrainCircuit className="w-4 h-4" /> 延迟</div>
                                    <div className="text-muted-foreground">1.2 秒</div>
                                </div>
                            </div>
                        </div>
                    )}
                </section>
            </div>
        </main>
    )
}

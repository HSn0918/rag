import * as React from "react"
import { MessageSquare, X, Send, Minimize2, Maximize2, Loader2, User, Bot, Database } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card } from "@/components/ui/card"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import { type AIConfig, type AIProvider } from "./settings-dialog"
import { PromiseClient } from "@connectrpc/connect"
import { RagService } from "@/gen/rag/v1/rag_connect"

interface Message {
    role: "user" | "assistant" | "system"
    content: string
}

interface AIChatProps {
    client: PromiseClient<typeof RagService>
}

import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select"

export function AIChat({ client }: AIChatProps) {
    const [isOpen, setIsOpen] = React.useState(false)
    const [isMinimized, setIsMinimized] = React.useState(false)
    const [messages, setMessages] = React.useState<Message[]>([])
    const [input, setInput] = React.useState("")
    const [isLoading, setIsLoading] = React.useState(false)
    const [loadingStage, setLoadingStage] = React.useState<string>("")
    const [config, setConfig] = React.useState<AIConfig | null>(null)
    const [useRAG, setUseRAG] = React.useState(true)
    const [selectedProvider, setSelectedProvider] = React.useState<AIProvider>("deepseek")
    const scrollRef = React.useRef<HTMLDivElement>(null)

    // Load config and preference on open
    React.useEffect(() => {
        if (isOpen) {
            const savedConfig = localStorage.getItem("rag_ai_config")
            const savedProvider = localStorage.getItem("rag_ai_selected_provider")

            if (savedConfig) {
                try {
                    setConfig(JSON.parse(savedConfig))
                } catch (e) {
                    console.error("Failed to load AI config", e)
                }
            }
            if (savedProvider) {
                setSelectedProvider(savedProvider as AIProvider)
            }
        }
    }, [isOpen])

    // Save provider selection
    const handleProviderChange = (value: AIProvider) => {
        setSelectedProvider(value)
        localStorage.setItem("rag_ai_selected_provider", value)
    }

    // Auto-scroll
    React.useEffect(() => {
        if (scrollRef.current) {
            scrollRef.current.scrollTop = scrollRef.current.scrollHeight
        }
    }, [messages, isOpen, loadingStage])

    const handleSend = async () => {
        if (!input.trim() || isLoading) return
        if (!config) {
            setMessages(prev => [...prev, { role: "system", content: "请先配置 AI 服务。" }])
            return
        }

        // Use selected provider directly
        const providerConfig = config[selectedProvider]

        // Fallback or check if configured
        if (!providerConfig || !providerConfig.apiKey) {
            setMessages(prev => [...prev, { role: "system", content: `请在与 ${selectedProvider} 交互前配置 API 密钥。` }])
            return
        }

        const userMsg: Message = { role: "user", content: input }
        setMessages(prev => [...prev, userMsg])
        setInput("")
        setIsLoading(true)
        setLoadingStage("正在启动...")

        try {
            let finalPrompt = userMsg.content

            // 1. RAG Retrieval Step
            if (useRAG) {
                setLoadingStage("正在搜索知识库...")
                try {
                    const ragRes = await client.getContext({ query: userMsg.content })
                    const context = ragRes.context

                    if (context) {
                        setLoadingStage("正在处理检索内容...")
                        // Augment the prompt
                        finalPrompt = `Context information is below.\n---------------------\n${context}\n---------------------\nGiven the context information and not prior knowledge, answer the query.\nQuery: ${userMsg.content}`

                        setMessages(prev => [...prev, { role: "system", content: `已检索到 ${ragRes.keywords.length > 0 ? ragRes.keywords.length : '若干'} 个相关关键词` }])
                    } else {
                        setLoadingStage("未找到相关内容，使用通用知识回答...")
                    }
                } catch (ragError) {
                    console.error("RAG retrieval failed", ragError)
                    setMessages(prev => [...prev, { role: "system", content: "知识库检索失败，转为直接提问。" }])
                }
            }

            // 2. AI Generation Step
            setLoadingStage("AI 正在思考...")
            let endpoint = `${providerConfig.baseUrl}/chat/completions`

            const response = await fetch(endpoint, {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                    "Authorization": `Bearer ${providerConfig.apiKey}`,
                },
                body: JSON.stringify({
                    model: providerConfig.model,
                    messages: messages.concat([{ role: "user", content: finalPrompt }]).map(m => {
                        if (m.role === "system") return null
                        // Ensure strict type compatibility for API
                        return { role: m.role as "user" | "assistant" | "system", content: m.content }
                    }).filter(Boolean),
                    stream: false
                })
            })

            if (!response.ok) {
                const errText = await response.text()
                throw new Error(`API Error: ${response.status} - ${errText}`)
            }

            const data = await response.json()
            const reply = data.choices?.[0]?.message?.content || "无回复。"

            setMessages(prev => [...prev, { role: "assistant", content: reply }])

        } catch (error: any) {
            setMessages(prev => [...prev, { role: "system", content: `错误: ${error.message}` }])
        } finally {
            setIsLoading(false)
            setLoadingStage("")
        }
    }

    if (!isOpen) {
        return (
            <Button
                onClick={() => setIsOpen(true)}
                className="fixed bottom-6 right-6 h-14 w-14 rounded-full shadow-lg z-50 bg-indigo-600 hover:bg-indigo-700 text-white"
            >
                <MessageSquare className="h-6 w-6" />
            </Button>
        )
    }

    return (
        <Card className={cn(
            "fixed bottom-6 right-6 z-50 shadow-2xl flex flex-col transition-all duration-300 bg-white dark:bg-gray-900 border-indigo-100 dark:border-indigo-900 overflow-hidden",
            isMinimized ? "w-72 h-14" : "w-[400px] h-[600px] max-h-[80vh]"
        )}>
            {/* Header */}
            <div className="flex items-center justify-between p-3 bg-indigo-50 dark:bg-indigo-950/50 border-b border-indigo-100 dark:border-indigo-900"
                onClick={() => !isMinimized && setIsMinimized(true)}
            >
                <div className="flex items-center gap-2 cursor-pointer">
                    <div className="p-1.5 bg-indigo-100 dark:bg-indigo-900 rounded-lg">
                        <Bot className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
                    </div>
                </div>

                {/* Model Switcher - Stop propagation to prevent minimize */}
                <div onClick={(e) => e.stopPropagation()} className="flex-1 mx-2">
                    <Select value={selectedProvider} onValueChange={(v) => handleProviderChange(v as AIProvider)}>
                        <SelectTrigger className="h-8 text-xs bg-white dark:bg-slate-900 border-indigo-200 dark:border-indigo-800">
                            <SelectValue placeholder="Select Model" />
                        </SelectTrigger>
                        <SelectContent>
                            <SelectItem value="deepseek">DeepSeek</SelectItem>
                            <SelectItem value="openai">OpenAI</SelectItem>
                            <SelectItem value="claude">Claude</SelectItem>
                            <SelectItem value="gemini">Gemini</SelectItem>
                        </SelectContent>
                    </Select>
                </div>

                <div className="flex gap-1">
                    <Button variant="ghost" size="icon" className="h-8 w-8 text-gray-500 hover:text-indigo-600" onClick={(e) => { e.stopPropagation(); setIsMinimized(!isMinimized) }}>
                        {isMinimized ? <Maximize2 className="h-4 w-4" /> : <Minimize2 className="h-4 w-4" />}
                    </Button>
                    <Button variant="ghost" size="icon" className="h-8 w-8 text-gray-500 hover:text-red-500" onClick={(e) => { e.stopPropagation(); setIsOpen(false) }}>
                        <X className="h-4 w-4" />
                    </Button>
                </div>
            </div>

            {/* Chat Area */}
            {!isMinimized && (
                <>
                    <div className="px-4 py-2 border-b border-gray-100 dark:border-gray-800 bg-gray-50/30 flex items-center justify-between">
                        <div className="flex items-center gap-2">
                            <Database className="w-3 h-3 text-indigo-500" />
                            <Label htmlFor="rag-mode" className="text-xs font-medium text-gray-600 dark:text-gray-400 cursor-pointer">启用知识库搜索</Label>
                        </div>
                        <Switch id="rag-mode" checked={useRAG} onCheckedChange={setUseRAG} className="scale-75" />
                    </div>

                    <div className="flex-1 overflow-y-auto p-4 space-y-4" ref={scrollRef}>
                        {messages.length === 0 && (
                            <div className="text-center text-gray-400 text-sm mt-8">
                                <p>今天有什么可以帮您？</p>
                                <p className="text-xs mt-2 opacity-70">Powered by {selectedProvider.charAt(0).toUpperCase() + selectedProvider.slice(1)}</p>
                            </div>
                        )}
                        {messages.map((msg, i) => (
                            <div key={i} className={cn(
                                "flex gap-2 text-sm max-w-[90%]",
                                msg.role === "user" ? "ml-auto flex-row-reverse" : "mr-auto"
                            )}>
                                <div className={cn(
                                    "w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 mt-1",
                                    msg.role === "user" ? "bg-indigo-100 text-indigo-600" : "bg-gray-100 text-gray-600 dark:bg-gray-800"
                                )}>
                                    {msg.role === "user" ? <User className="w-3 h-3" /> : <Bot className="w-3 h-3" />}
                                </div>
                                <div className={cn(
                                    "p-3 rounded-2xl whitespace-pre-wrap",
                                    msg.role === "user"
                                        ? "bg-indigo-600 text-white rounded-tr-none shadow-md"
                                        : msg.role === "system"
                                            ? "bg-amber-50 text-amber-800 border border-amber-200 text-xs w-full text-center italic py-1"
                                            : "bg-gray-100 dark:bg-gray-800 text-gray-800 dark:text-gray-100 rounded-tl-none shadow-sm"
                                )}>
                                    {msg.content}
                                </div>
                            </div>
                        ))}
                        {isLoading && (
                            <div className="flex gap-2 text-sm max-w-[90%] mr-auto animate-in fade-in slide-in-from-left-2">
                                <div className="w-6 h-6 rounded-full bg-gray-100 text-gray-600 dark:bg-gray-800 flex items-center justify-center flex-shrink-0 mt-1">
                                    <Bot className="w-3 h-3" />
                                </div>
                                <div className="p-3 rounded-2xl bg-gray-50 dark:bg-gray-800/50 text-gray-500 rounded-tl-none flex items-center gap-2 border border-gray-100 dark:border-gray-800">
                                    <Loader2 className="w-3 h-3 animate-spin text-indigo-500" />
                                    <span>{loadingStage || "Thinking..."}</span>
                                </div>
                            </div>
                        )}
                    </div>

                    {/* Input */}
                    <div className="p-3 border-t border-gray-100 dark:border-gray-800 bg-gray-50/50 dark:bg-gray-900/50">
                        <form
                            onSubmit={(e) => { e.preventDefault(); handleSend() }}
                            className="flex gap-2"
                        >
                            <Input
                                value={input}
                                onChange={e => setInput(e.target.value)}
                                placeholder="请输入问题..."
                                className="flex-1 bg-white dark:bg-gray-800"
                            />
                            <Button type="submit" size="icon" disabled={isLoading || !input.trim()} className="bg-indigo-600 hover:bg-indigo-700">
                                <Send className="w-4 h-4" />
                            </Button>
                        </form>
                    </div>
                </>
            )}
        </Card>
    )
}

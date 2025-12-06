"use client"

import * as React from "react"
import { List } from "react-window"
import { FileText, Calendar, Database, Trash2 } from "lucide-react"
import { RagService } from "@/gen/rag/v1/rag_connect"
import { createPromiseClient } from "@connectrpc/connect"
import { createConnectTransport } from "@connectrpc/connect-web"

const transport = createConnectTransport({
    baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api",
})
const client = createPromiseClient(RagService, transport)

interface Document {
    id: string
    title: string
    createdAt: string
    minioKey: string
}

interface DocumentListProps {
    refreshKey?: number
    optimisticDoc?: Document
}

export function DocumentList({ refreshKey = 0, optimisticDoc }: DocumentListProps) {
    const [documents, setDocuments] = React.useState<Document[]>([])
    const [isLoading, setIsLoading] = React.useState(false)
    const [cursor, setCursor] = React.useState<string>("")
    const [hasMore, setHasMore] = React.useState(true)
    const [deletingId, setDeletingId] = React.useState<string | null>(null)
    const loadingRef = React.useRef(false)
    const initialLoaded = React.useRef(false)
    const shouldResetRef = React.useRef(false)

    // Handle optimistic doc update
    React.useEffect(() => {
        if (optimisticDoc) {
            setDocuments(prev => {
                const exists = prev.some(d => d.id === optimisticDoc.id)
                if (exists) return prev
                return [optimisticDoc, ...prev]
            })
        }
    }, [optimisticDoc])

    // Initial load logic
    React.useEffect(() => {
        let active = true

        const resetState = async () => {
            if (active) {
                setCursor("")
                setHasMore(true)
                loadingRef.current = false
                initialLoaded.current = false
                shouldResetRef.current = true
            }
        }
        resetState()

        return () => { active = false }
    }, [refreshKey])

    React.useEffect(() => {
        let active = true
        if (initialLoaded.current) return

        const doLoad = async () => {
            if (loadingRef.current || isLoading || !hasMore) return
            loadingRef.current = true
            setIsLoading(true)
            try {
                const res = await client.listDocuments({
                    pageSize: 50,
                    cursor: ""
                })
                if (active) {
                    setDocuments(res.documents)
                    setCursor(res.nextCursor)
                    setHasMore(!!res.nextCursor)
                    initialLoaded.current = true
                    shouldResetRef.current = false
                }
            } catch (err) {
                console.error("Failed to list documents", err)
            } finally {
                if (active) {
                    setIsLoading(false)
                    loadingRef.current = false
                }
            }
        }

        doLoad()
        return () => { active = false }
    }, [refreshKey])

    const loadMore = React.useCallback(async () => {
        if (loadingRef.current || isLoading || !hasMore) return
        loadingRef.current = true
        setIsLoading(true)
        try {
            const res = await client.listDocuments({
                pageSize: 50,
                cursor: cursor
            })
            setDocuments(prev => {
                const seen = new Set(prev.map(d => d.id))
                const newDocs = res.documents.filter(d => !seen.has(d.id))
                return [...prev, ...newDocs]
            })
            setCursor(res.nextCursor)
            setHasMore(!!res.nextCursor)
        } catch (err) {
            console.error("Failed to list documents", err)
        } finally {
            setIsLoading(false)
            loadingRef.current = false
        }
    }, [cursor, hasMore, isLoading])

    const handleDelete = React.useCallback(async (id: string) => {
        if (deletingId) return
        const confirmed = window.confirm("确认删除该文档及其分块吗？")
        if (!confirmed) return

        setDeletingId(id)
        try {
            await client.deleteDocument({ documentId: id })
            setDocuments(prev => prev.filter(d => d.id !== id))
        } catch (err) {
            console.error("Failed to delete document", err)
            window.alert("删除失败，请稍后重试")
        } finally {
            setDeletingId(null)
        }
    }, [deletingId])

    const Row = ({ index, style }: { index: number, style: React.CSSProperties }) => {
        const doc = documents[index]
        if (!doc) {
            return (
                <div style={style} className="flex items-center justify-center text-sm text-gray-400">
                    Loading more...
                </div>
            )
        }
        return (
            <div style={style} className="px-4 py-2">
                <div className="flex items-center p-3 bg-white dark:bg-gray-800 rounded-lg border border-gray-100 dark:border-gray-700 hover:border-blue-200 transition-colors h-full">
                    <div className="mr-4 p-2 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
                        <FileText className="w-5 h-5 text-blue-600 dark:text-blue-400" />
                    </div>
                    <div className="flex-1 min-w-0">
                        <h4 className="font-medium text-gray-900 dark:text-gray-100 truncate">{doc.title}</h4>
                        <div className="flex items-center gap-4 mt-1 text-xs text-gray-500">
                            <span className="flex items-center gap-1">
                                <Database className="w-3 h-3" />
                                {doc.id.substring(0, 8)}...
                            </span>
                            <span className="flex items-center gap-1">
                                <Calendar className="w-3 h-3" />
                                {new Date(doc.createdAt).toLocaleDateString()}
                            </span>
                        </div>
                    </div>
                    <button
                        className="ml-4 inline-flex items-center gap-1 text-xs text-red-500 hover:text-red-600 disabled:opacity-50"
                        onClick={() => handleDelete(doc.id)}
                        disabled={deletingId === doc.id}
                    >
                        <Trash2 className="w-4 h-4" />
                        {deletingId === doc.id ? "删除中..." : "删除"}
                    </button>
                </div>
            </div>
        )
    }

    const handleRowsRendered = ({ visibleStopIndex }: { visibleStopIndex: number }) => {
        if (hasMore && !isLoading && visibleStopIndex >= documents.length - 10) {
            loadMore()
        }
    }

    const itemCount = hasMore ? documents.length + 1 : documents.length

    return (
        <div className="w-full h-[400px] bg-gray-50 dark:bg-gray-900/50 rounded-2xl border border-gray-200 dark:border-gray-800 overflow-hidden">
            <div className="p-4 border-b dark:border-gray-800 bg-white dark:bg-gray-900 flex justify-between items-center">
                <h3 className="font-semibold text-gray-900 dark:text-gray-100">Document Library</h3>
                <span className="text-xs text-gray-500">{documents.length} items</span>
            </div>
            {documents.length === 0 && !isLoading ? (
                <div className="h-full flex items-center justify-center text-gray-500">
                    No documents found
                </div>
            ) : (
                <List<any>
                    style={{ height: 340, width: "100%" }}
                    rowCount={itemCount}
                    rowHeight={80}
                    onRowsRendered={({ visibleStopIndex }: any) => handleRowsRendered({ visibleStopIndex })}
                    rowComponent={Row}
                    rowProps={{}}
                />
            )}
        </div>
    )
}

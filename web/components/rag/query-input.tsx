import * as React from "react"
import { Search, ArrowRight, Loader2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

interface QueryInputProps {
    onSearch: (query: string) => void
    isLoading: boolean
}

export function QueryInput({ onSearch, isLoading }: QueryInputProps) {
    const [query, setQuery] = React.useState("")

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault()
        if (query.trim()) {
            onSearch(query)
        }
    }

    return (
        <form onSubmit={handleSubmit} className="w-full max-w-2xl mx-auto relative group">
            <div className="relative flex items-center">
                <div className="absolute left-4 text-muted-foreground group-focus-within:text-primary transition-colors">
                    <Search className="w-5 h-5" />
                </div>
                <Input
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder="关于您的文档提问..."
                    className="pl-12 pr-32 h-14 text-lg rounded-full border-gray-200 dark:border-gray-800 shadow-sm focus-visible:ring-offset-2 transition-all group-hover:border-primary/50"
                    disabled={isLoading}
                />
                <div className="absolute right-2">
                    <Button
                        type="submit"
                        size="sm"
                        className="rounded-full h-10 px-4 transition-all hover:scale-105 active:scale-95"
                        disabled={!query.trim() || isLoading}
                    >
                        {isLoading ? (
                            <Loader2 className="w-4 h-4 animate-spin" />
                        ) : (
                            <ArrowRight className="w-4 h-4" />
                        )}
                    </Button>
                </div>
            </div>
        </form>
    )
}

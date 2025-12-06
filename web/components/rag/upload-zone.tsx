import * as React from "react"
import { UploadCloud, FileText, CheckCircle, Loader2 } from "lucide-react"
import { cn } from "@/lib/utils"
import { Progress } from "@/components/ui/progress"

interface UploadZoneProps {
    onUpload: (file: File) => void
    isUploading: boolean
    progress: number
    success: boolean
}

export function UploadZone({ onUpload, isUploading, progress, success }: UploadZoneProps) {
    const [isDragging, setIsDragging] = React.useState(false)
    const fileInputRef = React.useRef<HTMLInputElement>(null)

    const handleDragOver = (e: React.DragEvent) => {
        e.preventDefault()
        setIsDragging(true)
    }

    const handleDragLeave = (e: React.DragEvent) => {
        e.preventDefault()
        setIsDragging(false)
    }

    const handleDrop = (e: React.DragEvent) => {
        e.preventDefault()
        setIsDragging(false)
        if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
            validateAndUpload(e.dataTransfer.files[0])
        }
    }

    const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
        if (e.target.files && e.target.files.length > 0) {
            validateAndUpload(e.target.files[0])
        }
    }

    const validateAndUpload = (file: File) => {
        if (file.type === "application/pdf") {
            onUpload(file)
        } else {
            alert("Please upload a PDF file")
        }
    }

    return (
        <div
            className={cn(
                "relative group cursor-pointer overflow-hidden rounded-3xl border-2 border-dashed transition-all duration-300 ease-in-out",
                isDragging ? "border-primary bg-primary/5 scale-[1.02]" : "border-gray-200 dark:border-gray-800 hover:border-primary/50 hover:bg-gray-50 dark:hover:bg-gray-900/50",
                success ? "border-green-500 bg-green-50/50 dark:bg-green-900/20" : "",
                "max-w-2xl mx-auto h-64 flex flex-col items-center justify-center p-8 text-center"
            )}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => !isUploading && !success && fileInputRef.current?.click()}
        >
            <input
                type="file"
                ref={fileInputRef}
                className="hidden"
                accept=".pdf"
                onChange={handleFileSelect}
                disabled={isUploading || success}
            />

            {success ? (
                <div className="animate-in zoom-in spin-in-12 duration-500 flex flex-col items-center gap-4 text-green-600 dark:text-green-400">
                    <div className="p-4 bg-green-100 dark:bg-green-900/40 rounded-full">
                        <CheckCircle className="w-10 h-10" />
                    </div>
                    <p className="text-lg font-semibold">Ready to process!</p>
                </div>
            ) : isUploading ? (
                <div className="w-full max-w-xs space-y-4 animate-in fade-in zoom-in-95">
                    <div className="flex flex-col items-center gap-2">
                        <Loader2 className="w-8 h-8 text-primary animate-spin" />
                        <p className="text-sm font-medium text-muted-foreground">Analysing document...</p>
                    </div>
                    <Progress value={progress} className="h-2" />
                </div>
            ) : (
                <div className="space-y-4">
                    <div className="p-4 bg-primary/10 rounded-full w-fit mx-auto transition-transform group-hover:scale-110 duration-300">
                        <UploadCloud className="w-10 h-10 text-primary" />
                    </div>
                    <div className="space-y-1">
                        <h3 className="text-xl font-semibold tracking-tight">
                            Drop your PDF here
                        </h3>
                        <p className="text-sm text-muted-foreground">
                            or click to browse
                        </p>
                    </div>
                </div>
            )}
        </div>
    )
}

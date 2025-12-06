import type { Metadata } from 'next'
import './globals.css'
import { ClientProvider } from '@/components/client-provider'
import { cn } from '@/lib/utils'

export const metadata: Metadata = {
    title: 'RAG Visualization',
    description: 'Visualize your RAG pipeline',
}

export default function RootLayout({
    children,
}: {
    children: React.ReactNode
}) {
    return (
        <html lang="en" suppressHydrationWarning>
            <body className={cn("antialiased font-sans")}>
                <ClientProvider>
                    {children}
                </ClientProvider>
            </body>
        </html>
    )
}

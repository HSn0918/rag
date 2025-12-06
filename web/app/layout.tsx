import type { Metadata } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'
import { ClientProvider } from '@/components/client-provider'
import { cn } from '@/lib/utils'

const inter = Inter({ subsets: ['latin'] })

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
            <body className={cn(inter.className, "antialiased")}>
                <ClientProvider>
                    {children}
                </ClientProvider>
            </body>
        </html>
    )
}

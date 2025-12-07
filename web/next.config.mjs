/** @type {import('next').NextConfig} */
const nextConfig = {
    // Increase timeout for long-running RAG requests
    experimental: {
        proxyTimeout: 120000, // 2 minutes
    },
    httpAgentOptions: {
        keepAlive: true,
    },
    async rewrites() {
        const backendBase = process.env.BACKEND_URL ?? "http://localhost:8080"
        return [
            {
                source: "/api/:path*",
                destination: `${backendBase}/:path*`,
            },
        ]
    },
}

export default nextConfig

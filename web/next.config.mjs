/** @type {import('next').NextConfig} */
const nextConfig = {
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

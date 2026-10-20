/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async rewrites() {
    return [{ source: "/bff/:path*", destination: `${process.env.BOOKING_API_URL}/v1/:path*` }];
  },
};

export default nextConfig;

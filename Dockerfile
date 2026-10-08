# Builds and runs the ProxyConnectorBot on Bun (TypeScript, no build step).

FROM oven/bun:1.4-alpine AS deps
WORKDIR /app
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --production

FROM oven/bun:1.4-alpine
WORKDIR /app
ENV NODE_ENV=production

COPY --from=deps /app/node_modules ./node_modules
COPY package.json bun.lock ./
COPY src ./src

EXPOSE 8080
USER bun
ENTRYPOINT ["bun", "run", "src/index.ts"]

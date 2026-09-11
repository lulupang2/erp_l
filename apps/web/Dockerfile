FROM node:26.8.2-bookworm-slim AS build

ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN npm install --global pnpm@10.11.0
WORKDIR /workspace
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/package.json
RUN pnpm install --frozen-lockfile --filter @erp/web... --ignore-scripts
COPY apps/web apps/web
RUN pnpm --filter @erp/web build

FROM node:26.8.2-bookworm-slim
ENV NODE_ENV=production
ENV HOST=127.0.0.1
ENV PORT=3000
WORKDIR /app
COPY --from=build --chown=node:node /workspace/apps/web/build ./build
USER node
EXPOSE 3000
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=5 \
  CMD node -e "fetch('http://127.0.0.1:3000/items').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
CMD ["node", "build"]

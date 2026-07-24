ARG NODE_IMAGE=node:20.19.0-alpine
ARG NGINX_IMAGE=nginx:1.27.5-alpine
FROM ${NODE_IMAGE} AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

ENV PNPM_HOME=/pnpm \
    PATH=/pnpm:$PATH \
    VITE_BUILD_VERSION=${VERSION} \
    VITE_BUILD_COMMIT=${COMMIT} \
    VITE_BUILD_TIME=${BUILD_TIME} \
    VITE_SERVICE_BASE_URL=/ \
    VITE_OTHER_SERVICE_BASE_URL="{demo:'/'}"

RUN corepack enable && corepack prepare pnpm@10.5.0 --activate
WORKDIR /src/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
COPY frontend/packages ./packages
RUN --mount=type=cache,target=/pnpm/store pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

FROM ${NGINX_IMAGE} AS runtime
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY docker/proxy_params /etc/nginx/proxy_params
COPY --from=builder /src/frontend/dist /usr/share/nginx/html
EXPOSE 8080

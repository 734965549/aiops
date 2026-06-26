FROM node:20-alpine AS builder
WORKDIR /workspace

RUN npm install -g npm@11.6.2

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /workspace/dist /usr/share/nginx/html

EXPOSE 8080

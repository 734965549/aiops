FROM node:20-alpine AS builder
WORKDIR /workspace

RUN npm install -g npm@11.6.2

COPY package*.json ./
RUN npm ci

COPY . ./
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /workspace/dist /usr/share/nginx/html

EXPOSE 8080

# The optional hayat-console image: the React build behind nginx, which also
# proxies the admin API so the console and the API share one origin.
FROM node:22-alpine AS build
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

FROM nginx:1.27-alpine
# nginx's image renders templates/*.template with envsubst at start-up.
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
ENV HAYAT_ADMIN_URL=http://hayat-admin:8080
EXPOSE 80

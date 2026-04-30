# Stage 1: Build the Vue 3 SPA
FROM node:20-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Stage 2: Serve with Nginx
FROM nginx:1.27-alpine
RUN apk add --no-cache gettext  # provides envsubst
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/templates/default.conf.template

# HERMES_BACKEND_URL is substituted at container start via envsubst
ENV HERMES_BACKEND_URL=localhost:8080

# Use envsubst to inject HERMES_BACKEND_URL into nginx.conf at startup
CMD ["/bin/sh", "-c", "envsubst '${HERMES_BACKEND_URL}' < /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf && nginx -g 'daemon off;'"]

EXPOSE 80

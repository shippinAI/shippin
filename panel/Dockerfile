FROM node:20-alpine AS build
WORKDIR /app/panel

ARG PANEL_BASE_PATH=/
ENV PANEL_BASE_PATH=${PANEL_BASE_PATH}

COPY panel/package.json panel/package-lock.json* ./
RUN npm install

COPY panel/ ./
COPY api/openapi /app/api/openapi
RUN npm run build

FROM nginx:1.27-alpine
ARG PANEL_BASE_PATH=/
COPY panel/nginx.conf.template /tmp/default.conf.template
COPY --from=build /app/panel/dist /usr/share/nginx/html
RUN sed "s|__PANEL_BASE_PATH__|${PANEL_BASE_PATH}|g" /tmp/default.conf.template > /etc/nginx/conf.d/default.conf \
    && rm -f /tmp/default.conf.template
EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]

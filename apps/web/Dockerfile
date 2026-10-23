FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ARG VITE_KEYCLOAK_URL=http://localhost:18085
ARG VITE_KEYCLOAK_REALM=alumnibridge
ARG VITE_KEYCLOAK_CLIENT_ID=alumnibridge-web
ARG VITE_OIDC_CALLBACK_URI=http://localhost:5173/
RUN npm run build

FROM nginx:1.29-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80

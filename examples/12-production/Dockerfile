# Build context: this directory, for an app that requires a templatic
# release (see README for this checkout's replace).

# npm ci: the client libraries the bundle imports.
FROM node:26-slim AS npm
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

# generate, then build: the bundle and the binary.
FROM golang:1.27 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=npm /app/node_modules ./node_modules
RUN go tool templatic generate
RUN CGO_ENABLED=0 go build -trimpath -o /production .

# Run: the binary reads the bundle and the app's static files from its
# working directory (#47).
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /production /usr/local/bin/production
COPY --from=build /app/.templatic/dist ./.templatic/dist
COPY --from=build /app/public ./public
ENV ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/production"]

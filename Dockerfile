# One image for the server and every cmd/ tool. Container Apps Jobs run the
# same image with a different command, for example ["/app/ingest-ebird-recent"].

FROM node:lts-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/build ./web/build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ ./
COPY migrations/ ./migrations/
EXPOSE 8080
ENTRYPOINT ["/app/server"]

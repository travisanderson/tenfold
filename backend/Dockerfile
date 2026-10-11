# Build a static binary, then ship it on a minimal base image.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
# The platform sets PORT; 8080 is the local default.
EXPOSE 8080
ENTRYPOINT ["/api"]

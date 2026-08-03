FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/traccar-billing ./cmd/traccar-billing

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/traccar-billing /usr/local/bin/traccar-billing
EXPOSE 8083
ENTRYPOINT ["/usr/local/bin/traccar-billing"]

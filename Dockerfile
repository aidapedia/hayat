# The hayat image: one static binary that runs any role.
#   docker run ghcr.io/aidapedia/hayat serve --role=auth
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/aidapedia/hayat/internal/adminserver.Version=${VERSION}" \
    -o /out/hayat ./cmd/hayat

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hayat /usr/local/bin/hayat
EXPOSE 8443 8080 7443 9090
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/hayat"]
CMD ["serve"]

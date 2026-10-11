FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -o /out/import-xlsx ./cmd/import-xlsx

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/api /api
COPY --from=build /out/import-xlsx /import-xlsx
COPY migrations /migrations
EXPOSE 8080
ENTRYPOINT ["/api"]

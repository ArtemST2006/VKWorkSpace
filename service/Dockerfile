FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ghreport ./cmd/ghreport

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ghreport /ghreport
USER 65532:65532
ENTRYPOINT ["/ghreport"]

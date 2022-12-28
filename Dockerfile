FROM golang:1.19 as build
WORKDIR /app

COPY go.* /app
RUN go mod download
COPY . /app
RUN CGO_ENABLED=0 go build -o gh-rate-limit-exporter main.go

FROM gcr.io/distroless/static-debian11:nonroot
USER nobody
COPY --from=build --chown=nobody:nobody /app/gh-rate-limit-exporter /
CMD [ "/gh-rate-limit-exporter" ]
FROM golang:1.19-alpine as build
WORKDIR /app
COPY . /app
RUN go mod download
RUN CGO_ENABLED=0 go build -o gh-rate-limit-exporter main.go

FROM gcr.io/distroless/static
USER nobody
COPY --from=build --chown=nobody:nobody /app/gh-rate-limit-exporter /
CMD [ "/gh-rate-limit-exporter" ]
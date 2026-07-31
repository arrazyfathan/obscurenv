FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/obv-backend .

FROM alpine:3.22
RUN adduser -D -H obv
USER obv
COPY --from=build /out/obv-backend /usr/local/bin/obv-backend
EXPOSE 8080
ENTRYPOINT ["obv-backend"]


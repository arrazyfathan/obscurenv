FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/obe-backend .

FROM alpine:3.22
RUN adduser -D -H obe
USER obe
COPY --from=build /out/obe-backend /usr/local/bin/obe-backend
EXPOSE 8080
ENTRYPOINT ["obe-backend"]

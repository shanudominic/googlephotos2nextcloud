
# Use official Golang image to create a binary
FROM golang:1.24 as builder

WORKDIR /app

# Copy the rest of the code and build
COPY . .
WORKDIR /app/src
RUN go mod tidy
RUN go build -o /media2nextcloud

# Use a minimal image for the final container
FROM gcr.io/distroless/base-debian12

COPY --from=builder /media2nextcloud /media2nextcloud

ENTRYPOINT ["/media2nextcloud"]

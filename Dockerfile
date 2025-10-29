# Use the official Golang image as the build environment
FROM golang:latest AS builder

# Set environment variables to ensure a statically linked binary
ENV CGO_ENABLED=0 \
    GOOS=linux

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application code
COPY . .

# Set the working directory to where main.go is located
WORKDIR /app/cmd

# Build the Go app
RUN go build -o /conversly main.go

# Use a minimal image for the runtime environment
FROM alpine:latest

# Install necessary CA certificates (optional, but recommended)
RUN apk --no-cache add ca-certificates

# Set the working directory inside the container
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /conversly .

# Create cmd directory and ensure proper permissions
RUN mkdir -p /app/cmd && chmod +x ./conversly

CMD ["./conversly"]
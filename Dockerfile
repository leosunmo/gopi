FROM golang:1.13.5-alpine3.10 as builder

# Install git + SSL ca certificates.
# Git is required for fetching the dependencies.
# Ca-certificates is required to call HTTPS endpoints.
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

ARG GOPROXY

# Create gopi user
RUN adduser -D -g '' gopi
WORKDIR /gopi
COPY . .

# Build the binary
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOPROXY=${GOPROXY} go build -ldflags="-w -s" -o /go/bin/gopi

############################

# Replace with Scratch in prod
FROM alpine:3.10 

# Import from builder.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd

# Copy our static executable
COPY --from=builder /go/bin/gopi /go/bin/gopi

# Copy the templates until they get embedded in binary
COPY --from=builder /gopi/templates /go/bin/templates

# Use an unprivileged user.
USER gopi

WORKDIR /go/bin/

ENTRYPOINT ["./gopi"]
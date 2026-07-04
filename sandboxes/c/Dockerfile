FROM alpine:3.21

RUN apk add --no-cache gcc musl-dev libc-dev make
RUN adduser --home /sandbox --disabled-password sandbox

USER sandbox
WORKDIR /sandbox

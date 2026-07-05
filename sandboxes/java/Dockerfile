FROM alpine:3.21

RUN apk add --no-cache openjdk21-jdk
RUN adduser --home /sandbox --disabled-password sandbox

USER sandbox
WORKDIR /sandbox

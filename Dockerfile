FROM debian:jessie-slim
RUN apt-get update && apt-get install -y ca-certificates && apt-get clean
ADD linux-bufferlinks /
ENTRYPOINT ["/linux-bufferlinks"]
EXPOSE ":19870"

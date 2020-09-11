FROM gcr.io/jenkinsxio/jx-cli-base:latest

ENTRYPOINT ["jx-preview"]

COPY ./build/linux/jx-preview /usr/bin/jx-preview
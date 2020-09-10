FROM gcr.io/jenkinsxio/jx-cli-base:0.0.21

ENTRYPOINT ["jx-preview"]

COPY ./build/linux/jx-preview /usr/bin/jx-preview
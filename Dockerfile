FROM gcr.io/jenkinsxio-labs-private/jxl-base:0.0.55

ENTRYPOINT ["jx-preview"]

COPY ./build/linux/jx-preview /usr/bin/jx-preview
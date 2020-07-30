FROM centos:7

RUN yum install -y git

ENTRYPOINT ["jx-preview"]

COPY ./build/linux/jx-preview /usr/bin/jx-preview
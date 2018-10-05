FROM alpine
LABEL maintainer="senthilrch <senthilrch@gmail.com>"

COPY fledged /opt/bin/fledged
RUN chmod 755 /opt/bin/fledged

ENTRYPOINT ["/opt/bin/fledged"]

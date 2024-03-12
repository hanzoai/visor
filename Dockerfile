FROM casbin/guacd:1.5.4 as guacd
FROM casbin/dbgate:latest as dbgate
FROM node:18.19.0 AS FRONT
WORKDIR /web
COPY ./web .
RUN yarn install --frozen-lockfile --network-timeout 1000000 && yarn run build


FROM golang:1.20.12 AS BACK
WORKDIR /go/src/casvisor
COPY . .
RUN chmod +x ./build.sh
RUN ./build.sh


FROM alpine:latest AS STANDARD
LABEL MAINTAINER="https://casvisor.org/"
ARG USER=casvisor

RUN sed -i 's/https/http/' /etc/apk/repositories \
    && apk --no-cache add sudo \
        curl \
        ca-certificates  \
        nodejs  \
    && update-ca-certificates

WORKDIR /home/$USER
COPY --from=BACK --chown=$USER:$USER /go/src/casvisor/server ./server
COPY --from=BACK --chown=$USER:$USER /go/src/casvisor/data ./data
COPY --from=BACK --chown=$USER:$USER /go/src/casvisor/entrypoint.sh ./entrypoint.sh
COPY --from=BACK --chown=$USER:$USER /go/src/casvisor/conf/app.conf ./conf/app.conf
COPY --from=FRONT --chown=$USER:$USER /web/build ./web/build
COPY --from=dbgate --chown=$USER:$USER /home/dbgate-docker ./dbgate-docker

RUN chmod +x ./entrypoint.sh

RUN adduser -D $USER -u 1000 \
    && echo "$USER ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/$USER \
    && chmod 0440 /etc/sudoers.d/$USER \
    && mkdir logs \
    && chown -R $USER:$USER /home/casvisor/dbgate-docker \
    && chown -R $USER:$USER /home/casvisor/logs

USER $USER

EXPOSE 3000
EXPOSE 19000

ENTRYPOINT ["/bin/sh"]
CMD ["/home/casvisor/entrypoint.sh"]


FROM guacd AS ALLINONE
LABEL MAINTAINER="https://casvisor.org/"

WORKDIR /home/casvisor

USER root
RUN apt-get update \
    && apt-get install -y      \
        nodejs                 \
        mariadb-server         \
        mariadb-client         \
        ca-certificates        \
    && update-ca-certificates  \
    && rm -rf /var/lib/apt/lists/*

COPY --from=BACK /go/src/casvisor/server ./server
COPY --from=BACK /go/src/casvisor/data ./data
COPY --from=BACK /go/src/casvisor/docker-entrypoint.sh ./docker-entrypoint.sh
COPY --from=BACK /go/src/casvisor/conf/app.conf ./conf/app.conf
COPY --from=FRONT /web/build ./web/build
COPY --from=dbgate /home/dbgate-docker ./dbgate-docker

EXPOSE 19000
EXPOSE 3000

ENTRYPOINT ["/bin/bash"]
CMD ["/home/casvisor/docker-entrypoint.sh"]

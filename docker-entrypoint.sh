#!/bin/bash
if [ "${MYSQL_ROOT_PASSWORD}" = "" ] ;then MYSQL_ROOT_PASSWORD=123456 ;fi

service mariadb start

mysqladmin -u root password ${MYSQL_ROOT_PASSWORD}

/opt/guacamole/sbin/guacd -b 0.0.0.0 -L "$GUACD_LOG_LEVEL"

HOST_DOMAIN="dockerhost"
ping -q -c1 $HOST_DOMAIN > /dev/null 2>&1
if [ $? != 0 ]
then
  HOST_IP=$(ip route | awk 'NR==1 {print $3}')
  echo "$HOST_IP $HOST_DOMAIN" >> /etc/hosts
fi

node /home/dbgate-docker/bundle.js --listen-api &
exec /home/casvisor/server --createDatabase=true

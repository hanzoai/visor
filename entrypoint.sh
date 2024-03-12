#!/bin/sh
node /home/casvisor/dbgate-docker/bundle.js --listen-api &
exec /home/casvisor/server --createDatabase=true

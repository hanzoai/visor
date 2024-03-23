#!/bin/sh
node /home/dbgate-docker/bundle.js --listen-api &
exec /home/casvisor/server --createDatabase=true

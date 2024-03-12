#!/bin/bash
set -x

cp -r /opt/input /tmp/source-app
/usr/bin/mvn "$@" 
cp -r /tmp/source-app/input /opt

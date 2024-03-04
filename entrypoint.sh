#!/bin/bash
set -x

cp -r /opt/input/source /tmp/source-code
sed -i 's|/opt/input/source|/tmp/source-code|g' /opt/input/config/settings.json
/usr/bin/konveyor-analyzer "$@" 
sed -i 's|/tmp/source-code|/opt/input/source|g' /opt/input/config/settings.json

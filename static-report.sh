#!/bin/bash
set -x

HOMEDIR=""

if [ "$(uname)" == "Linux" ] || [ "$(uname)" == "Darwin" ]; then
  HOMEDIR="$HOME"
else
  # windows
  HOMEDIR="%USERPROFILE%"
fi

# TODO test this on windows
(cd ${HOMEDIR}/.kantra/static-report && 
npm clean-install &&
CI=true PUBLIC_URL=. npm run build &&
cp ${HOMEDIR}/.kantra/static-report/public/output.js ${HOMEDIR}/.kantra/static-report/build/output.js &&
rm -rf ${HOMEDIR}/.kantra/static-report/static-report &&
mv ${HOMEDIR}/.kantra/static-report/build ${HOMEDIR}/.kantra/static-report/static-report)

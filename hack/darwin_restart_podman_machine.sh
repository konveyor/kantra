#!/bin/sh

# Default variables can be overriden from environment
: ${VM_NAME="kantra"}
: ${MEM=4096}
: ${CPUS=4}
: ${DISK_SIZE=100}

# See https://github.com/konveyor/kantra/issues/91
# See https://github.com/containers/podman/issues/16106#issuecomment-1317188581
ulimit -n unlimited
podman machine stop $VM_NAME
podman machine rm $VM_NAME -f
podman machine init $VM_NAME -v $HOME:$HOME -v /private/tmp:/private/tmp -v /var/folders/:/var/folders/
podman machine set $VM_NAME --cpus $CPUS --memory $MEM --disk-size $DISK_SIZE
podman system connection default $VM_NAME
podman machine start $VM_NAME


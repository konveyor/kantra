Once you have kantra installed, these examples will help you run both an
analyze and a transform command.

### Analyze

- Get the example application to run analysis on
`git clone https://github.com/konveyor/example-applications`

- List available target technologies
`kantra analyze --list-targets`

- Run analysis with a specified target technology
`kantra analyze --input=<path-to/example-applications/example-1> --output=<path-to-output-dir> --target=cloud-readiness`

- Several analysis reports will have been created in your specified output path:

```sh
$ ls ./output/ -1
analysis.log
dependencies.yaml
dependency.log
output.yaml
static-report
```

`output.yaml` is the file that contains issues report.
`static-report` contains the static HTML report.
`dependencies.yaml`contains a dependencies report.

### Transform

- Get the example application to transform source code
`git clone https://github.com/ivargrimstad/jakartaee-duke`

- View available OpenRewrite recipes
`kantra transform openrewrite --list-targets`

- Run a recipe on the example application
`kantra transform openrewrite --input=<path-to/jakartaee-duke> --target=jakarta-imports`

- Inspect the `jakartaee-duke` application source code diff to see the transformation

### Asset Generation

#### Discover

- View supported source platform resources
`kantra discover --list-platforms`

- Print YAML representations of source platform resources
`kantra discover cloud-foundry --input=<path-to/manifest-yaml>`

    For example:
    `kantra discover cloud-foundry --input=./test-data/asset_generation/discover/cf-sample-app.yaml`

- Output YAML representations of source platform resources in the output directory
`kantra discover cloud-foundry --input=<path-to/manifest-yaml> --output-dir=<path-to/output-dir>`

    For example:
    `kantra discover cloud-foundry --input=./test-data/asset_generation/discover/cf-sample-app.yaml --output-dir=/tmp/output-dir`

- Perform discovery and separate sensitive data (credentials, secrets) into a dedicated file:
`kantra discover cloud-foundry --input=<path-to/manifest-yaml> --conceal-sensitive-data=true --output-dir=<path-to/output-dir>`

    For example:
    `kantra discover cloud-foundry --input=./test-data/asset_generation/discover/cf-sample-app.yaml --conceal-sensitive-data=true --output-dir=/tmp/output-dir`

- Perform a live discover and print the YAML representation of source platform resources
`kantra discover cloud-foundry --use-live-connection --spaces=<space1,space2>`

    For example:
    `kantra discover cloud-foundry --use-live-connection --spaces=space1,space2`

- Perform a live discover and output the YAML representations of source platform
  resources in the output directory
`kantra discover cloud-foundry --use-live-connection --spaces=<space1,space2> --output-dir=<path-to/output-dir>`

    For example:
    `kantra discover cloud-foundry --use-live-connection --spaces=space1,space2 --output-dir=/tmp/output-dir`

- Perform a live discover of a specific application and output the YAML representations of source platform resources in the output directory:
`kantra discover cloud-foundry --use-live-connection --spaces=<space1,space2> --app-name=<app-name> --output-dir=<path-to/output-dir>`

    For example:
    `kantra discover cloud-foundry --use-live-connection --spaces=space1,space2 --app-name=my-app --output-dir=/tmp/output-dir`

- Perform live discovery and separate sensitive data (credentials, secrets) into a dedicated file:
`kantra discover cloud-foundry --use-live-connection --spaces=<space1,space2> --conceal-sensitive-data=true --output-dir=<path-to/output-dir>`

    For example:
    `kantra discover cloud-foundry --use-live-connection --spaces=space1,space2 --conceal-sensitive-data=true --output-dir=/tmp/output-dir`

#### Generate

- Output the Helm template manifests
`kantra generate helm --input=<path-to/discover-manifest> --chart-dir=<path-to/helm-chart>`

    For example:
    `kantra generate helm --chart-dir=./test-data/asset_generation/helm/k8s_only --input=./test-data/asset_generation/helm/discover.yaml`

- Print the Helm template manifests
`kantra generate helm --input=<path/to/discover/manifest> --chart-dir=<path/to/helmchart> --output-dir=<path-to/output-dir>`

    For example:
    `kantra generate helm --chart-dir=./test-data/asset_generation/helm/k8s_only --input=./test-data/asset_generation/helm/discover.yaml --output-dir=/tmp/generate-dir`


### Running as a Tekton Task
- Create an SCC with required permissions for podman to run within another container
```
cat << EOF | oc create -f -
# Based on https://docs.openshift.com/pipelines/latest/secure/unprivileged-building-of-container-images-using-buildah.html
kind: SecurityContextConstraints
metadata:
  annotations:
  name: rootless-in-pod
allowHostDirVolumePlugin: false
allowHostIPC: false
allowHostNetwork: false
allowHostPID: false
allowHostPorts: false
allowPrivilegeEscalation: true
allowPrivilegedContainer: false
allowedCapabilities:
# Allow usage of the MKNOD capability to create devices otherwise
# Error: crun: mknod `/dev/full`: Operation not permitted: OCI permission denied
- MKNOD
# Allow usage of the SETFCAP capability so that we can unpack newuidmap / newgidmap binaries which have extend attributes
# otherwise errors out with
# lsetxattr /usr/bin/newgidmap: operation not permitted exit status 1"
- SETFCAP
# Allow usage of the SYS_ADMIN capability to mount `proc` and other filesystems, otherwise crun errors out with
# Error: crun: mount `proc` to `proc`: Permission denied: OCI permission denied
- SYS_ADMIN
apiVersion: security.openshift.io/v1
defaultAddCapabilities: null
fsGroup:
  type: MustRunAs
groups:
- system:cluster-admins
readOnlyRootFilesystem: false
requiredDropCapabilities:
- KILL
# Needed to avoid "no subuid ranges found for user \"1001200000\" # in /etc/subuid" error
# and podman not finding a $HOME directory for storing initial config
runAsUser:
  type: MustRunAs
  uid: 1000
# Allow Pods to by pass SeLinux Confinement
# needed to mount `proc` otherwise crun bails out with
# Error: crun: mount `proc` to `proc`: Permission denied: OCI permission denied
# See also "Rootless Podman without the privileged flag" in https://www.redhat.com/sysadmin/podman-inside-kubernetes
seLinuxContext:
  type: RunAsAny
supplementalGroups:
  type: RunAsAny
users: []
volumes:
- configMap
- downwardAPI
- emptyDir
- persistentVolumeClaim
- projected
- secret
EOF
```

```
oc create -n konveyor-tackle serviceaccount podman
```

```
oc adm policy add-scc-to-user -n konveyor-tackle rootless-in-pod -z podman
```

- Create a Tekton Task
```
cat << EOF | oc create -f -
apiVersion: tekton.dev/v1 # or tekton.dev/v1beta1
kind: Task
metadata:
  name: kantra-cli
  namespace: konveyor-tackle
spec:
  steps:
    - name: kantra-cli
      image: quay.io/konveyor/kantra:latest
      command:
        - bash
      args:
        - -c
        - kantra analyze --input /workspace/code/ --output output/ --run-local=false --overwrite
      volumeMounts:
        - name: containersstorage
          mountPath: /workspace/code
        - name: var-lib-container
          mountPath: /var/lib/containers/
        - name: run-containers
          mountPath: /run/containers/
EOF
```

- Create a Tekton TaskRun to run it
```
cat << EOF | oc create -f -
apiVersion: tekton.dev/v1 # or tekton.dev/v1beta1
kind: TaskRun
metadata:
  name: kantra-cli
  namespace: konveyor-tackle
spec:
  serviceAccountName: podman
  taskRef:
    name: kantra-cli
  podTemplate:
    env:
    - name: HOME
      value: /home/mta
    securityContext:
      seLinuxOptions:
          type: spc_t
    volumes:
      - name: containersstorage
        emptyDir:
          medium: ""
      - name: var-lib-container
        emptyDir:
          medium: ""
      - name: run-containers
        emptyDir:
          medium: ""
EOF
```

- To make this more useful replace emptyDir storage with workspaces/PVCs containing code and preserving results and adapt the TaskRun to a PipelineRun if it better suits your workflow.

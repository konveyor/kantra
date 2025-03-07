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

- Output a YAML representation of source platform resources  
`kantra discover --input=<path-to/manifest-yaml>`

    For example:  
    `./kantra discover cloud-foundry --input=./test-data/asset_generation/discover/cf-sample-app.yaml`

- Print a YAML representation of source platform resources  
`kantra discover --input=<path-to/manifest-yaml> --output=<path-to/ouput-file>`

    For example:  
    `kantra discover cloud-foundry --input=./test-data/asset_generation/discover/cf-sample-app.yaml --output=/tmp/output.yaml`

#### Generate

- Output the Helm template manifests  
`kantra generate helm --input=<path-to/discover-manifest> --chart-dir=<path-to/helm-chart>`
    
    For example:  
    `kantra generate helm --chart-dir=./test-data/asset_generation/helm/k8s_only --input=./test-data/asset_generation/helm/discover.yaml`

- Print the Helm template manifests  
`kantra generate helm --input=<path/to/discover/manifest> --chart-dir=<path/to/helmchart> --output-dir=<path-to/output-dir>`
    
    For example:  
    `kantra generate helm --chart-dir=./test-data/asset_generation/helm/k8s_only --input=./test-data/asset_generation/helm/discover.yaml --output-dir=/tmp/generate-dir`

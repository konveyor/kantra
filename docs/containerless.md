## Test and Run Containerless Kantra

### Clone the requirements:

```sh
git clone https://github.com/eemcmullan/containerless-kantra-deps.git
```

## Move them to where kantra will look for packged binaries and default rulesets:

```sh
mv $HOME/containerless-kantra-deps $HOME/.kantra
```

### From kantra, run:

```sh
go run main.go analyze-bin  --input <java-app> --output <output-dir> --rules <java-rules>
```

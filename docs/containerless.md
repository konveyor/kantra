# Run Containerless Kantra

Have OpenJDK 17+ and Maven installed

Have $JAVA_HOME set.

## Download kantra and requirements:

Download appropriate zip for your OS [here](https://github.com/konveyor/kantra/releases/tag/v0.6.0-alpha.2) 

## Move kantra binary to your $PATH:

```sh
mv $HOME/kantra.<os>.<arch>/<os>-kantra /usr/bin
```

### Move requirements to kantra known location, or run kantra from the current directory:
*Note:* kantra will first look for these requirements in the current dir, and fall back to the path below.


```sh
mv $HOME/kantra.<os>.<arch> $HOME/.kantra
```

## Run analysis:
Kantra will default to running containerless analysis. To run analysis in containers, use the `--run-local=false` option.

```sh
kantra analyze  --input <java-app> --output <output-dir> --rules <java-rules>
```

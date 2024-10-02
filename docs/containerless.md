# Run Containerless Kantra

Have OpenJDK 17+ and Maven installed

## Download kantra and requirements:

Download appropriate zip for your OS [here](https://github.com/konveyor/kantra/releases/tag/v0.6.0-alpha.1) 

## Move kantra binary to your $PATH:

```sh
mv $HOME/kantra.<os>.<arch>/<os>-kantra /usr/bin
```

### Move requirements to kantra known location:

```sh
mv $HOME/kantra.<os>.<arch> $HOME/.kantra
```

## Run analysis:

```sh
kantra analyze-bin  --input <java-app> --output <output-dir> --rules <java-rules>
```

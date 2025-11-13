# Run Containerless Kantra

Requirements:
- Have OpenJDK 17+ and Maven installed.
- Have $JAVA_HOME set.
- **For Gradle analysis**:
  - Have OpenJDK 8 installed.
  - Have $JAVA8_HOME set and pointing to the OpenJDK 8 home.
  - The project should have a Gradle wrapper.

## Download kantra and requirements:

Download appropriate zip for your OS [here](https://github.com/konveyor/kantra/releases)

## Move kantra binary to your $PATH:

```sh
mv $HOME/kantra.<os>.<arch>/<os>-kantra /usr/local/bin
```

### Move requirements to kantra known location, or run kantra from the current directory:
*Note:* kantra will first look for these requirements in the current dir, and fall back to the path below.


```sh
mv $HOME/kantra.<os>.<arch> $HOME/.kantra
```

## Run analysis:
Kantra defaults to containerless mode. To run in hybrid mode (providers in containers), use `--run-local=false`.

```sh
kantra analyze  --input <java-app> --output <output-dir> --rules <java-rules>
```

# Kantra project skills

## Testing kantra analyze

These skills are for **testing code changes and reproducing bugs** for the `analyze` command: running analysis so the user can reproduce an issue or validate a fix. Reference this file when the user asks to test, validate, reproduce a bug, or verify analysis. Show the command run and the analysis result (output dir path). **For hybrid analysis runs (skills 2, 3, 4)**: also show the **CONTAINER_TOOL** and image env vars used: **RUNNER_IMG**, **JAVA_PROVIDER_IMG**, **GENERIC_PROVIDER_IMG** (and **CSHARP_PROVIDER_IMG** if set).

**Input**: Every skill requires the user to provide the application path (the app where they see the bug or want to reproduce behavior). There is no default input. Use the path the user gives for `--input`.

**Analysis options**: Do not use default `--target` or any other analysis CLI options. The user may add options when they provide input (e.g. `--target`, `--source`, `--rules`, `--mode`, `--label-selector`, `--profile-dir`, etc.). Include only the options the user specifies; otherwise use only `--input`, `--output`, `--overwrite`, and for hybrid skills `--run-local=false`.

**If the user asks for testing or to reproduce a bug but does not provide an application path**: Notify them that an input application path is required. Do not run an analyze command until they supply it.

**When the user asks to test a specific release** (e.g. release-0.9): Assume the user is already on that branch. Build kantra from the current tree. If testing hybrid mode, use that release tag and set `RUNNER_IMG` to it. Do not run `git checkout`. **Inform the user** that release testing assumes they are on the requested release branch (e.g. `release-0.9`); if they are not, they should checkout that branch first and then re-run the test.

**When the user requests a hybrid-mode test (skill 2, 3, or 4)**: You **must** run the [Prerequisites for hybrid mode](#prerequisites-for-hybrid-mode-skills-2-3-4) steps **first**, in order—pull images; for a release branch, assume the user is on that branch and build the kantra image with that tag from the current tree, then set `RUNNER_IMG` (for main/latest, build with `:latest`); set provider env vars—then run the analyze command. Do not skip or assume these steps are already done.

---

## Prerequisites for hybrid mode (skills 2, 3, 4)

**Run these steps in order before every hybrid-mode analyze.** Do not run `./kantra analyze --run-local=false` until these are complete.

1. **Pull the provider and analyzer images**. Use `:latest` for main, or a release tag (e.g. `:release-0.9`) to test a release branch. If the user does not specify, use latest, and inform the user of the images being pulled:

   ```bash
   # Replace TAG with latest or e.g. release-0.9. Use docker instead of podman if user prefers Docker; default to podman.
   podman pull quay.io/konveyor/analyzer-lsp:TAG
   podman pull quay.io/konveyor/jdtls-server-base:latest
   podman pull quay.io/konveyor/java-external-provider:TAG
   podman pull quay.io/konveyor/generic-external-provider:TAG
   ```
   (For release branch: e.g. `quay.io/konveyor/analyzer-lsp:release-0.9`, `quay.io/konveyor/java-external-provider:release-0.9`, etc.)

2. **Build the kantra image**, then set `RUNNER_IMG` to that built image. The kantra image tag must match what you are testing:

   - **If testing main / latest**: Build with `:latest` and set `RUNNER_IMG` to it:
     ```bash
     podman build -t quay.io/konveyor/kantra:latest .
     export RUNNER_IMG=quay.io/konveyor/kantra:latest
     ```

   - **If testing a release branch** (e.g. `release-0.9`): Assume the user is already on that branch. Build the kantra image from the current tree with that tag, then set `RUNNER_IMG` to that image. Inform the user that release testing assumes they are on the requested release branch.

     ```bash
     podman build -t quay.io/konveyor/kantra:release-0.9 .
     export RUNNER_IMG=quay.io/konveyor/kantra:release-0.9
     ```
     Use the same tag for the image and for `RUNNER_IMG` (e.g. `release-0.9` → `quay.io/konveyor/kantra:release-0.9`).

   (Use `docker` instead of `podman` if the user prefers Docker; default to podman. If using Docker: `export CONTAINER_TOOL=$(which docker)` before the build.)

3. **Set provider image env vars** (from `cmd/settings.go`). Use the same TAG as in step 1 (e.g. `latest` or `release-0.9`):
   ```bash
   export JAVA_PROVIDER_IMG=quay.io/konveyor/java-external-provider:TAG
   export GENERIC_PROVIDER_IMG=quay.io/konveyor/generic-external-provider:TAG
   ```
   For C# testing also: `export CSHARP_PROVIDER_IMG=quay.io/konveyor/c-sharp-provider:TAG`

4. **Only then** run the hybrid analyze command from skill 2, 3, or 4 (with the user's `--input` and any options they specified).

---

## 1. Test local containerless Java app

**When to use**: User wants to test analysis of a Java app in containerless mode (no containers).

**Command** (user provides `<path-to-application>` for `--input`; user may add analysis options such as `--target`, `--source`, `--rules`):
```bash
./kantra analyze \
  --input <path-to-application> \
  --output ./out-containerless-java \
  --overwrite
```

**Result**: Show the command as run and `ls -la ./out-containerless-java/`. No comparison to other files.

---

## 2. Test hybrid mode Java app

**When to use**: User wants to test analysis with the Java provider running in a container (hybrid mode).

**Before this command**: Execute the [Prerequisites for hybrid mode](#prerequisites-for-hybrid-mode-skills-2-3-4) in full (pull images → build kantra image with the appropriate tag—for a release branch, assume user is on that branch and use that tag—then set `RUNNER_IMG` → set `JAVA_PROVIDER_IMG` and `GENERIC_PROVIDER_IMG`). Then run:

**Command** (user provides `<path-to-application>` for `--input`; user may add analysis options such as `--target`, `--source`, `--rules`):
```bash
./kantra analyze \
  --input <path-to-application> \
  --output ./out-hybrid-java \
  --run-local=false \
  --overwrite
```

**Result**: Show **CONTAINER_TOOL** and image env vars (**RUNNER_IMG**, **JAVA_PROVIDER_IMG**, **GENERIC_PROVIDER_IMG**), the command as run, and `ls -la ./out-hybrid-java/` (and optionally a short peek at `output.yaml`). No comparison to other files.

---

## 3. Test Python provider (hybrid mode)

**When to use**: User wants to test analysis of a Python app. Python is only supported in hybrid mode.

**Before this command**: Execute the [Prerequisites for hybrid mode](#prerequisites-for-hybrid-mode-skills-2-3-4) in full (pull images → build kantra image with the appropriate tag—for a release branch, assume user is on that branch and use that tag—then set `RUNNER_IMG` → set provider env vars). Then run:

**Command** (user provides `<path-to-application>` for `--input`; user may add analysis options such as `--target`, `--source`, `--rules`):
```bash
./kantra analyze \
  --input <path-to-application> \
  --output ./out-hybrid-python \
  --run-local=false \
  --overwrite
```

**Result**: Show **CONTAINER_TOOL** and image env vars (**RUNNER_IMG**, **JAVA_PROVIDER_IMG**, **GENERIC_PROVIDER_IMG**), the command as run, and `ls -la ./out-hybrid-python/` (and optionally a short peek at `output.yaml`). No comparison to other files.

---

## 4. Test Node.js provider (hybrid mode)

**When to use**: User wants to test analysis of a Node.js app. Node.js is only supported in hybrid mode.

**Before this command**: Execute the [Prerequisites for hybrid mode](#prerequisites-for-hybrid-mode-skills-2-3-4) in full (pull images → build kantra image with the appropriate tag—for a release branch, assume user is on that branch and use that tag—then set `RUNNER_IMG` → set provider env vars). Then run:

**Command** (user provides `<path-to-application>` for `--input`; user may add analysis options such as `--target`, `--source`, `--rules`):
```bash
./kantra analyze \
  --input <path-to-application> \
  --output ./out-hybrid-nodejs \
  --run-local=false \
  --overwrite
```

**Result**: Show **CONTAINER_TOOL** and image env vars (**RUNNER_IMG**, **JAVA_PROVIDER_IMG**, **GENERIC_PROVIDER_IMG**), the command as run, and `ls -la ./out-hybrid-nodejs/` (and optionally a short peek at `output.yaml`). No comparison to other files.

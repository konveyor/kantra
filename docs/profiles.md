# Profiles

## Overview

Profiles in Kantra provide a way to define reusable analysis configurations that can be applied to applications. They allow you to standardize analysis settings, rules, and scope across multiple projects or share configurations between team members. Profiles are particularly useful when working with the Konveyor Hub, where they can be centrally managed and synchronized across different environments.

## What are Profiles?

A profile is a YAML configuration file that defines:

- **Analysis Rules**: Which rulesets and label selectors to apply
- **Analysis Scope**: Whether to include dependency analysis, known libraries, and package filtering
- **Hub Integration**: Connection to Konveyor Hub for centralized management

Profiles eliminate the need to specify complex command-line arguments repeatedly and ensure consistent analysis configurations across your organization.

## Profile Structure

```yaml
id: 1
createUser: admin
createTime: 2025-11-12T23:57:35
name: Profile-1
mode:
  withDeps: true
scope:
  withKnownLibs: true
  packages:
    included:
    - one
    - two
    excluded:
    - three
    - four
rules:
  targets:
  - id: 1
    name: Application server migration 
  - id: 2
    name: Containerization
  labels:
    included:
    - konveyor.io/target=spring6
    - konveyor.io/source=springboot
    - konveyor.io/target=quarkus
    excluded:
    - C
    - D
  files:
  - id: 400
    name: ""
  repository:
    kind: git
    url: <url>
    branch: ""
    tag: ""
    path: default/generated
```

### Profile Fields

- **id**: Unique identifier for the profile
- **createUser**: User who created the profile
- **createTime**: Timestamp when the profile was created
- **name**: Human-readable name for the profile

#### Mode Configuration
- **withDeps**: Whether to include dependency analysis

#### Scope Configuration
- **withKnownLibs**: Include analysis of known libraries
- **packages**: Package filtering configuration
  - **included**: Array of packages to include in analysis
  - **excluded**: Array of packages to exclude from analysis

#### Rules Configuration
- **targets**: Array of target configurations for migration
  - **id**: Target identifier
  - **name**: Target name/description
- **labels**: Label selector configuration
  - **included**: Array of labels to include (e.g., "konveyor.io/target=spring6")
  - **excluded**: Array of labels to exclude
- **files**: File-based rule configurations
  - **id**: File rule identifier
  - **name**: File rule name
- **repository**: External ruleset repository configuration
  - **kind**: Repository type (e.g., "git")
  - **url**: Repository URL
  - **branch**: Git branch (optional)
  - **tag**: Git tag (optional)
  - **path**: Path within repository to rulesets

## Configuration Commands

Kantra provides a `config` command with subcommands for managing profiles and Hub integration.

### Main Config Command

```bash
kantra config [flags]
```

**Flags:**
- `--list-profiles <path>`: List local Hub profiles in the specified application directory
- `--insecure, -k`: Skip TLS certificate verification for Hub connections

**Example:**
```bash
# List profiles in current application
kantra config --list-profiles /path/to/my-app

# List profiles with insecure connection
kantra config --list-profiles /path/to/my-app --insecure
```

### Hub Login

Connect Kantra to a Konveyor Hub instance and store credentials in `~/.kantra/auth.json`:

```bash
kantra config login [host]
```

**Arguments:**
- `host` (optional): Hub **API base URL** (e.g. `https://hub.example.com` or, on OpenShift, `https://<route>/hub`). If omitted, you are prompted for the host.

**Flags:**
- `--insecure, -k`: Skip TLS certificate verification (use for clusters with self-signed certificates)

#### Authentication methods

Kantra supports two ways to authenticate at login time:

1. **OIDC device flow (default, interactive terminal)**  
   Kantra starts OIDC login, prints a verification URL and user code, and waits while you sign in in the browser. After authentication, Kantra creates a Hub **API key** (personal access token) and saves it to `auth.json`.

2. **Existing PAT via `HUB_TOKEN`**  
   Set a personal access token from the Hub UI (or API) before running login:
   ```bash
   export HUB_TOKEN="<your-hub-pat>"
   kantra config login https://hub.example.com/hub
   ```
   Kantra validates the token and stores it in `auth.json` without running the browser flow.

On non-interactive terminals (no TTY), Kantra prompts for a PAT instead of starting OIDC.

#### Stored credentials

After a successful login, `~/.kantra/auth.json` contains:

```json
{
  "host": "https://hub.example.com/hub",
  "token": "<api-key-or-pat>"
}
```

The `token` field is the credential used by `kantra config sync` and other Hub commands. `HUB_TOKEN` is only read during `config login`; later commands use `auth.json`.

**Examples:**
```bash
# OIDC device login (prompt for host if omitted)
kantra config login

# OIDC device login with hub URL
kantra config login https://hub.example.com/hub

# Login with self-signed / untrusted TLS
kantra config --insecure login https://hub.example.com/hub

# Login with an existing PAT from the environment
export HUB_TOKEN="$(cat my-pat.txt)"
kantra config --insecure login https://hub.example.com/hub
```

### Profile Synchronization

After `kantra config login`, sync analysis profiles from the Hub for an application:

```bash
kantra config sync [flags]
```

**Lookup (one required):**
- `--url <repository-url>`: Git repository URL of the application in the Hub. Use `url:branch` to match a specific branch (e.g. `https://github.com/org/repo.git:main`).
- `--binary <name>`: Application binary identifier in the Hub (for binary-based applications; use with `--profile-path`).

**Optional flags:**
- `--application-path <path>`: Local directory where profile bundles are extracted (for `--url`; defaults to the current directory).
- `--profile-path <path>`: Download directory when using `--binary` (required with `--binary`).
- `--host <hub-url>`: Hub API base URL. If it matches the host in `auth.json`, stored credentials are used. If omitted, sync uses `auth.json` only. If set to a different host without stored credentials, Kantra uses OIDC and may prompt via the browser when the Hub returns unauthorized responses.
- `--insecure, -k`: Set on the parent `config` command to skip TLS verification for all Hub traffic in that invocation.

Repository URLs are matched flexibly (with or without a `.git` suffix and trailing slashes). If the Hub filter returns no results, Kantra lists applications and matches by repository URL.

**Examples:**
```bash
# Sync using credentials from login (recommended)
kantra config --insecure sync \
  --url https://github.com/mycompany/my-app.git \
  --application-path /path/to/my-app

# Same hub URL as login; --host is optional
kantra config --insecure sync \
  --host https://hub.example.com/hub \
  --url https://github.com/mycompany/my-app.git \
  --application-path /path/to/my-app

# Binary application
kantra config sync --binary my-app-binary --profile-path /path/to/profiles
```


## Using Profiles

### Creating a Profile Directory

Profiles are stored in a `.konveyor/profiles` directory within your application.
Each profile should be in its own subdirectory with a `profile.yaml` file.
For example: `.konveyor/profiles/profile-1/profile.yaml`.

**Directory Structure:**
```
my-application/
├── src/
├── .konveyor/
│   └── profiles/
│       ├── profile-1/
│       │   └── profile.yaml
│       └── profile-2/
│           └── profile.yaml
└── other-files...
```

If a single profile is found in `.konveyor/profiles/`, it will be used by default for analysis configuration options. You can also use the `kantra analyze --profile` option to pass in a valid profile path.


### Running Analysis with a Profile

Use the `--profile-dir` flag to specify the profile directory:

```bash
kantra analyze --profile-dir myapp/.konveyor/profiles
```

When using a profile, the following flags will override settings on a profile:
- `--input` (derived from profile location)
- `--mode` (set based on `withDeps`)
- `--analyze-known-libraries` (set based on `withKnownLibs`)
- `--label-selector` (set based on `labels.included`)
- `--source` and `--target` (derived from labels)
- `--rules` (set based on `rules.repository` and `rules.files`)
- `--incident-selector` (set based on `scope.packages`)

## Profile Management Commands

### List Local Profiles

List profiles for a specific application:

```bash
kantra config list --profile-dir /path/to/application
```

### Sync Profiles from Hub

Synchronize profiles from the Hub (requires `kantra config login` or `HUB_TOKEN` at login time):

```bash
kantra config sync --url <repository-url> --application-path <path-to-application>
```


### Example: End-to-End Hub Workflow

This example demonstrates a complete workflow from Hub login to running analysis with a synced profile.

#### Step 1: Login to the Hub

Authenticate with your Konveyor Hub instance (OIDC device flow in an interactive terminal):

```bash
kantra config --insecure login https://hub.myapp.com/hub
```

You will see a verification URL and user code. Open the URL in a browser, sign in if asked, then enter the code on the device verification page. Kantra creates an API key and saves it to `~/.kantra/auth.json`.

Alternatively, use a PAT from the Hub UI:

```bash
export HUB_TOKEN="<pat-from-hub>"
kantra config --insecure login https://hub.myapp.com/hub
```

#### Step 2: Navigate to Your Application

```bash
cd /path/to/my-java-app
```

#### Step 3: Sync Profile from Hub

Sync profiles for the application repository (uses `auth.json`; pass `--insecure` on `config` if needed):

```bash
kantra config --insecure sync \
  --url https://github.com/mycompany/my-app-repo.git \
  --application-path /path/to/my-java-app
```

Use the same repository URL as registered on the Hub application (with or without `.git` is fine). This downloads profile bundles into `.konveyor/profiles/` under the application path.

#### Step 4: Verify Downloaded Profile

Check what profile was downloaded:

```bash
ls .konveyor/profiles/

cat .konveyor/profiles/profile-1/profile.yaml
```


#### Step 5: Run Analysis with Profile

Since the profile is in the default location (`.konveyor/profiles/`), you can run analysis without specifying the profile path:

```bash
kantra analyze --output <output-dir>
```

Or explicitly specify the profile directory:

```bash
kantra analyze --profile-dir .konveyor/profiles --output <output-dir>
```


## Troubleshooting

### Common Issues

1. **Profile directory not found**
   ```
   Error: failed to stat profile at path /path/to/profiles
   ```
   - Ensure the profile directory exists
   - Check that the path is correct and accessible
   - Verify the `.konveyor/profiles` directory structure

2. **Authentication required**
   ```
   Error: Hub authentication required for sync: no stored Hub authentication found
   ```
   - Run `kantra config login` (OIDC) or set `HUB_TOKEN` and run login again
   - Confirm `~/.kantra/auth.json` exists with `host` and `token`
   - For `config sync --host`, use the same Hub base URL as in `auth.json`, or omit `--host` after login

3. **Hub connection / authorization errors**
   ```
   Error: hub authentication failed: ...
   GET /hub/applications failed: 403 ... missing credentials
   ```
   - Run `kantra config login` so a PAT is stored in `auth.json`
   - If using `config sync --host`, ensure it matches the logged-in host or omit `--host`
   - Without stored credentials, `--host` uses OIDC; the Hub must return 401 for the binding to start device login (some deployments return 403 instead)

4. **Application not found in Hub**
   ```
   Error: no applications found in Hub for given input
   ```
   - Confirm the application exists in the Hub with a **repository** URL (not binary-only)
   - Use the repository URL shown on the Hub application (`--url`); Kantra matches with or without `.git`
   - Try the URL exactly as shown in the Hub UI

5. **TLS certificate issues**
   ```
   Error: x509: certificate signed by unknown authority
   ```
   - Pass `--insecure` on the parent `config` command for both login and sync:
     `kantra config --insecure login ...` and `kantra config --insecure sync ...`
   - Or add the cluster CA to your system trust store

6. **API key / PAT expiration**
   ```
   Error: hub authentication failed: ... 401 ...
   ```
   - API keys created at login expire after **168 hours** (Kantra default)
   - Run `kantra config login` again to obtain a new key, or set `HUB_TOKEN` to a PAT you manage in the Hub UI

7. **OIDC device login**
   - Visit the printed `/oidc/device` URL; sign in on `/oidc/login` if redirected, then enter the user code on the verification page
   - Keep the `config login` process running until it completes
   - If login fails after a Hub restart, run `config login` again (device authorizations are not persisted across Hub restarts)


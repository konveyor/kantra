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

To connect Kantra to a Konveyor Hub instance, use the login subcommand:

```bash
kantra config login [host] [username] [password]
```

**Arguments (all optional):**
- `host`: Hub URL (e.g., `https://hub.example.com`)
- `username`: Your Hub username  
- `password`: Your Hub password

**Flags:**
- `--insecure, -k`: Skip TLS certificate verification

If arguments are not provided, you'll be prompted interactively for:
- **Hub URL**: The URL of your Konveyor Hub instance
- **Username**: Your Hub username
- **Password**: Your Hub password (entered securely, hidden from terminal)

**Examples:**
```bash
# Interactive login (prompts for all credentials)
kantra config login

# Provide all credentials as arguments
kantra config login https://hub.example.com myuser mypassword

# Provide only host, prompt for username/password
kantra config login https://hub.example.com

# Login with insecure connection (skip TLS verification)
kantra config login --insecure
```

### Profile Synchronization

Once logged in, you can sync profiles from the Hub using the sync subcommand:

```bash
kantra config sync --url <repository-url> [flags]
```

**Required Flags:**
- `--url <repository-url>`: URL of the remote application repository to sync profiles for

**Optional Flags:**
- `--application-path <path>`: Path to the local application directory (defaults to current directory)
- `--insecure, -k`: Skip TLS certificate verification (inherited from parent config command)

**Examples:**
```bash
# Sync profiles for a repository to current directory
kantra config sync --url https://github.com/mycompany/my-app.git

# Sync profiles to specific application path
kantra config sync --url https://github.com/mycompany/my-app.git --application-path /path/to/my-app

# Sync with insecure connection
kantra config sync --url https://github.com/mycompany/my-app.git --insecure
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

Synchronize profiles from the Hub (requires login):

```bash
kantra config sync --url <repository-url> --application-path <path-to-application>
```


### Example: End-to-End Hub Workflow

This example demonstrates a complete workflow from hub login to running analysis with a synced profile.

#### Step 1: Login to the Hub

First, authenticate with your Konveyor Hub instance:

```bash
kantra config login
```

You'll be prompted for:

```
Host: https://hub.myapp.com
Username: myusername
Password: [hidden]
```

#### Step 2: Navigate to Your Application

```bash
cd /path/to/my-java-app
```

#### Step 3: Sync Profile from Hub

Sync the available profiles for your application from the Hub:

```bash
kantra config sync --url https://github.com/mycompany/my-app-repo.git --application-path /path/to/my-java-app
```

This downloads profiles associated with the application repository to `.konveyor/profiles/`.

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
   Error: no stored authentication found, please login
   ```
   - Run `kantra config login` to authenticate with the Hub
   - Check that `~/.kantra/auth.json` exists and contains valid tokens

3. **Hub connection issues**
   ```
   Error: login failed with status: 401 Unauthorized
   ```
   - Verify Hub URL is correct and accessible
   - Check username and password
   - Ensure Hub is accessible from your network
   - Try using `--insecure` flag if using self-signed certificates

4. **Application not found in Hub**
   ```
   Error: no applications found in Hub for URL: https://github.com/example/repo.git
   ```
   - Verify the repository URL is correct and matches exactly what's configured in the Hub
   - Ensure the application exists in the Hub and is associated with the repository
   - Check that you have access permissions to the application in the Hub

5. **TLS certificate issues**
   ```
   Error: x509: certificate signed by unknown authority
   ```
   - Use the `--insecure` flag to skip TLS verification: `kantra config login --insecure`
   - Or properly configure your system's certificate store with the Hub's CA certificate

6. **Token expiration**
   ```
   Error: stored authentication is invalid. Please login
   ```
   - Re-authenticate with `kantra config login`
   - Tokens are automatically refreshed, but may need manual re-login in some cases


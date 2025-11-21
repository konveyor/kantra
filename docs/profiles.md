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

### Hub Login

To connect Kantra to a Konveyor Hub instance, use the login command:

```bash
kantra config login
```

This will prompt you for:
- **Hub URL**: The URL of your Konveyor Hub instance (e.g., `https://hub.example.com`)
- **Username**: Your Hub username
- **Password**: Your Hub password (entered securely)

The login process:
1. Validates the Hub URL format
2. Sends authentication credentials to the Hub's `/api/login` endpoint
3. Stores authentication tokens for future API calls
4. Enables profile synchronization and sharing features

### Profile Synchronization

Once logged in, you can:
- **Download profiles** from the Hub to your local environment
- **Sync profile updates** to keep configurations current

## Using Profiles

### Creating a Profile Directory

Profiles are stored in a `.konveyor/profiles` directory within your application.
Each profile should be in its own subdirectory with a `profile.yaml` file.
For example: `.konveyor/profiles/profile-1/profile.yaml`.
If a single profile is found here, it will be used by default for analysis configuration
options. You can also use the `kantra analyze --profile` option to pass in a valid
profile path.


### Running Analysis with a Profile

Use the `--profile` flag to specify the profile directory:

```bash
kantra analyze --profile myapp/.konveyor/profiles
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
kantra config --list-profiles /path/to/application
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
URL: https://hub.myapp.com
username: <username>
password: 12345
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
kantra analyze --profile .konveyor/profiles --output <output-dir>
```


## Troubleshooting

### Common Issues

1. **Profile directory not found**
   ```
   Error: failed to stat profile at path /path/to/profiles
   ```
   - Ensure the profile directory exists
   - Check that the path is correct and accessible

3. **Hub connection issues**
   ```
   Error: login failed with status: 401 Unauthorized
   ```
   - Verify Hub URL is correct
   - Check username and password
   - Ensure Hub is accessible from your network


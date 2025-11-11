# Kantra Profiles

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
apiVersion: v1
kind: Profile
metadata:
  name: "profile-name"
  id: "unique-profile-id"
  source: "hub-or-local"
  syncedAt: "2023-01-01T00:00:00Z"
  version: "1.0.0"
spec:
  rules:
    labelSelectors:
      - "konveyor.io/target=cloud-readiness"
      - "konveyor.io/source=java"
    rulesets:
      - "custom-ruleset.yaml"
      - "/path/to/another/ruleset.yaml"
    useDefaultRules: true
    withDepRules: false
  scope:
    depAanlysis: true
    withKnownLibs: false
    packages: "com.example"
  hubMetadata:
    applicationId: "app-123"
    profileId: "profile-456"
    readonly: true
```

### Profile Fields

#### Metadata
- **name**: Human-readable name for the profile
- **id**: Unique identifier (optional, used for Hub integration)
- **source**: Source of the profile (hub, local, etc.)
- **syncedAt**: Timestamp of last synchronization with Hub
- **version**: Profile version

#### Rules Configuration
- **labelSelectors**: Array of label selector expressions to filter rules
- **rulesets**: Array of paths to custom ruleset files
- **useDefaultRules**: Whether to include Kantra's default rulesets
- **withDepRules**: Whether to include dependency analysis rules

#### Scope Configuration
- **depAanlysis**: Enable/disable dependency analysis
- **withKnownLibs**: Include analysis of known libraries
- **packages**: Package filter expression (e.g., "com.example" to analyze only packages starting with com.example)

#### Hub Metadata (Optional)
- **applicationId**: Associated application ID in the Hub
- **profileId**: Profile ID in the Hub
- **readonly**: Whether the profile is read-only

## Hub Integration

### What is the Konveyor Hub?

The Konveyor Hub is a centralized platform for managing application modernization projects. It provides:

- **Centralized Profile Management**: Store and share analysis profiles across teams
- **Application Tracking**: Monitor multiple applications and their modernization progress
- **Collaboration**: Share analysis results and configurations with team members
- **Governance**: Enforce consistent analysis standards across your organization

### Hub Login

To connect Kantra to a Konveyor Hub instance, use the login command:

```bash
kantra config --login
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
- **Upload local profiles** to share with your team
- **Sync profile updates** to keep configurations current

## Using Profiles

### Creating a Profile Directory

Profiles are stored in a `.konveyor/profiles` directory within your application.
If a profile is found here, it will be used by default for analysis configuration
options. You can also use the `kantra analyze --profile` option to pass in a valid
profile path.


### Running Analysis with a Profile

Use the `--profile` flag to specify the profile directory:

```bash
kantra analyze --profile myapp/.konveyor/profiles
```

When using a profile, the following flags will override settings on a profile:
- `--input` (derived from profile location)
- `--mode` (set based on `depAanlysis`)
- `--analyze-known-libraries` (set based on `withKnownLibs`)
- `--label-selector` (set based on `labelSelectors`)
- `--source` and `--target` (derived from label selectors)
- `--rules` (set based on `rulesets`)
- `--enable-default-rulesets` (set based on `useDefaultRules`)
- `--no-dependency-rules` (set based on `withDepRules`)
- `--incident-selector` (set based on `packages`)

## Profile Management Commands

### List Local Profiles

List profiles for a specific application:

```bash
kantra config --list-profiles /path/to/application
```

### Sync Profiles from Hub

Synchronize profiles from the Hub (requires login):

```bash
kantra config --sync application-id
```

## Examples

### Example 1: End-to-End Hub Workflow

This example demonstrates a complete workflow from hub login to running analysis with a synced profile.

#### Step 1: Login to the Hub

First, authenticate with your Konveyor Hub instance:

```bash
kantra config --login
```

You'll be prompted for:
```
URL: https://hub.mycompany.com
username: john.doe
password: [hidden]
Login successful
```

#### Step 2: Navigate to Your Application

```bash
cd /path/to/my-java-app
```

#### Step 3: Sync Profile from Hub

Sync the available profiles for your application from the Hub:

```bash
kantra config --sync my-app-123
```

This downloads profiles associated with application ID `my-app-123` to `.konveyor/profiles/`.

#### Step 4: Verify Downloaded Profile

Check what profile was downloaded:

```bash
ls .konveyor/profiles/
# Output: java-modernization.yaml

cat .konveyor/profiles/java-modernization.yaml
```

Example synced profile:
```yaml
apiVersion: v1
kind: Profile
metadata:
  name: "Java Modernization Standard"
  id: "java-mod-001"
  source: "hub"
  syncedAt: "2023-12-01T10:30:00Z"
  version: "1.2.0"
spec:
  rules:
    labelSelectors:
      - "konveyor.io/source=java"
      - "konveyor.io/target=cloud-readiness"
    useDefaultRules: true
    withDepRules: false
  scope:
    depAanlysis: true
    withKnownLibs: false
  hubMetadata:
    applicationId: "my-app-123"
    profileId: "java-mod-001"
    readonly: true
```

#### Step 5: Run Analysis with Profile

Since the profile is in the default location (`.konveyor/profiles/`), you can run analysis without specifying the profile path:

```bash
kantra analyze
```

Or explicitly specify the profile directory:

```bash
kantra analyze --profile .konveyor/profiles
```



### Example 2: Cloud Readiness Assessment

```yaml
# .konveyor/profiles/cloud-readiness.yaml
apiVersion: v1
kind: Profile
metadata:
  name: "Cloud Readiness Assessment"
spec:
  rules:
    labelSelectors:
      - "konveyor.io/target=cloud-readiness"
    useDefaultRules: true
    withDepRules: true
  scope:
    depAanlysis: true
    withKnownLibs: true
```

**Usage:**
```bash
kantra analyze --profile .konveyor/profiles
```


### Example 4: Hub-Managed Profile

```yaml
# .konveyor/profiles/enterprise-standards.yaml
apiVersion: v1
kind: Profile
metadata:
  name: "Enterprise Analysis Standards"
  id: "ent-std-001"
  source: "hub"
  syncedAt: "2023-12-01T10:30:00Z"
  version: "2.1.0"
spec:
  rules:
    labelSelectors:
      - "konveyor.io/target=kubernetes"
      - "enterprise.io/compliance=required"
    useDefaultRules: true
    withDepRules: true
  scope:
    depAanlysis: true
    withKnownLibs: true
  hubMetadata:
    applicationId: "myapp-123"
    profileId: "profile-ent-std-001"
    readonly: true
```

## Best Practices

### 1. Profile Organization
- Use descriptive names that indicate the analysis purpose
- Group related profiles in the same directory
- Version your profiles when making significant changes

### 2. Label Selector Strategy
- Use specific label selectors to target relevant rules
- Combine source and target labels for migration scenarios
- Test label selectors with `kantra analyze --list-targets` and `--list-sources`

### 3. Hub Integration
- Regularly sync profiles to get the latest updates
- Use readonly profiles for organization-wide standards
- Create application-specific profiles for custom requirements

### 4. Rule Management
- Start with default rules and add custom rulesets as needed
- Document custom rulesets and their purpose
- Test rule combinations before deploying to production

### 5. Scope Configuration
- Enable dependency analysis for comprehensive coverage
- Use package filtering to focus on relevant code
- Consider performance impact of analyzing known libraries

## Troubleshooting

### Common Issues

1. **Profile directory not found**
   ```
   Error: failed to stat profile at path /path/to/profiles
   ```
   - Ensure the profile directory exists
   - Check that the path is correct and accessible

2. **Invalid profile YAML**
   ```
   Error: failed to unmarshal profile file
   ```
   - Validate YAML syntax
   - Check that all required fields are present
   - Ensure proper indentation

3. **Hub connection issues**
   ```
   Error: login failed with status: 401 Unauthorized
   ```
   - Verify Hub URL is correct
   - Check username and password
   - Ensure Hub is accessible from your network

4. **Conflicting flags**
   ```
   Error: input must not be set when profile is set
   ```
   - Remove conflicting command-line flags when using profiles
   - Let the profile configure analysis settings

### Validation

To validate a profile before using it:

1. Check YAML syntax:
   ```bash
   yamllint .konveyor/profiles/myprofile.yaml
   ```

2. Test the profile:
   ```bash
   kantra analyze --profile .konveyor/profiles --dry-run
   ```

3. Verify rule selection:
   ```bash
   kantra analyze --profile .konveyor/profiles --list-targets
   ```


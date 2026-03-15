## Commands

### Analyze

- `./kantra analyze --input=<path/to/source> --output=<path/to/output> --overwrite` — Run analysis (containerless mode by default)

- `./kantra analyze --input=... --output=... --run-local=false` — Run analysis in hybrid mode (providers in containers)

### Config 

- `./kantra config login [host] [user] [password]` — Login to Konveyor Hub and store authentication tokens in `~/.kantra/auth.json`

- `./kantra config sync --url=<repo-url> --application-path=<local/app/path>` — Sync Hub application profiles. Downloads profile bundles to `.konveyor/profiles/` in the current directory (default) or `--application-path`

- `./kantra config list [--profile-dir=<path>]` — List local Hub profiles in an application directory (defaults to current directory)
- Add `--insecure` to any config command to skip TLS certificate verification

### Transform openrewrite

- `./kantra transform openrewrite --input=<path> --target=<target>` — Run OpenRewrite recipes

### Test

- `./kantra test --rules=<path> [options]` — Test YAML rules
